#!/usr/bin/env python3
"""GARDA load test: 2 sites x 5 guards (10 guards), tablets + web users (TAQ-13).

Standard library only (Python 3.9+). Talks to a running stack through nginx
(https://<host>/api/v1) and to Keycloak through nginx (/auth).

  loadtest.py --creds creds.json --state state.json setup
  loadtest.py --creds creds.json --state state.json run --minutes 60 --report out/

creds.json (keep it OUT of the repo; it holds passwords and TOTP secrets):
  {"<realm>": {"admin": {"username": "...", "password": "...", "totp": "<raw secret>"},
               "supervisor": {...}, "hr": {...}, "client": {...}}, ...}
  "totp" is the secret Keycloak shows on the OTP setup page (hidden input
  `totpSecret`), the key is its UTF-8 bytes (see totp()).

What it does (per guard thread; scan pace is compressed vs. a real shift):
  guard PIN check -> clock-in -> patrol (start, scan every --scan-every s, end)
  repeated; every few patrols an incident with photo and a visitor with exit;
  tablet health beacon, roster/routes refresh; web users (admin, supervisor, hr,
  client per realm) reload the dashboard page set (parallel) and poll live data;
  every --sync-every minutes all guards replay an offline queue (a patrol of
  10 scans + incident + photo + visitor, ~16 requests) at the same time.
Pass criteria (TAQ-13): dashboard page set < 2 s, sync queue < 10 s, no 5xx or
429 or timeouts over the run. The report is written as report.json + report.md.
"""
import argparse, base64, hashlib, hmac, html, http.client, http.cookiejar, json, os, random, re
import secrets, ssl, struct, subprocess, sys, threading, time, urllib.parse, urllib.request, uuid
from concurrent.futures import ThreadPoolExecutor

HOST = os.environ.get("LT_HOST", "localhost")
CA = os.environ.get("LT_CA", os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "nginx", "certs", "ca.pem"))
CTX = ssl.create_default_context(cafile=CA)
# 64x64 JPEG with EXIF/Photoshop markers: the media pipeline decodes it and strips the metadata
JPEG = base64.b64decode(
    "/9j/4AAQSkZJRgABAQAASABIAAD/4QBMRXhpZgAATU0AKgAAAAgAAYdpAAQAAAABAAAAGgAAAAAAA6ABAAMAAAABAAEAAKACAAQA"
    "AAABAAAAQKADAAQAAAABAAAAQAAAAAD/7QA4UGhvdG9zaG9wIDMuMAA4QklNBAQAAAAAAAA4QklNBCUAAAAAABDUHYzZjwCyBOmA"
    "CZjs+EJ+/8AAEQgAQABAAwEiAAIRAQMRAf/EAB8AAAEFAQEBAQEBAAAAAAAAAAABAgMEBQYHCAkKC//EALUQAAIBAwMCBAMFBQQE"
    "AAABfQECAwAEEQUSITFBBhNRYQcicRQygZGhCCNCscEVUtHwJDNicoIJChYXGBkaJSYnKCkqNDU2Nzg5OkNERUZHSElKU1RVVldY"
    "WVpjZGVmZ2hpanN0dXZ3eHl6g4SFhoeIiYqSk5SVlpeYmZqio6Slpqeoqaqys7S1tre4ubrCw8TFxsfIycrS09TV1tfY2drh4uPk"
    "5ebn6Onq8fLz9PX29/j5+v/EAB8BAAMBAQEBAQEBAQEAAAAAAAABAgMEBQYHCAkKC//EALURAAIBAgQEAwQHBQQEAAECdwABAgMR"
    "BAUhMQYSQVEHYXETIjKBCBRCkaGxwQkjM1LwFWJy0QoWJDThJfEXGBkaJicoKSo1Njc4OTpDREVGR0hJSlNUVVZXWFlaY2RlZmdo"
    "aWpzdHV2d3h5eoKDhIWGh4iJipKTlJWWl5iZmqKjpKWmp6ipqrKztLW2t7i5usLDxMXGx8jJytLT1NXW19jZ2uLj5OXm5+jp6vLz"
    "9PX29/j5+v/bAEMAAgICAgICAwICAwUDAwMFBgUFBQUGCAYGBgYGCAoICAgICAgKCgoKCgoKCgwMDAwMDA4ODg4ODw8PDw8PDw8P"
    "D//bAEMBAgICBAQEBwQEBxALCQsQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEP/dAAQA"
    "BP/aAAwDAQACEQMRAD8A8Xooor8fP9GAooooAKKKKACiiigD/9Dxeiiivx8/0YCiiigAooooAKKKKAP/0fF6KKK/Hz/RgKKKKACi"
    "iigAooooA//S8Xooor8fP9GAooooAKKKKACiiigD/9k=")

# ── metrics ────────────────────────────────────────────────────────────────
LOCK = threading.Lock()
SAMPLES = []   # (name, status, seconds)
ERRORS = []    # (time, name, status, detail)
EVENTS = {"sync": [], "dashboard": [], "docker": []}


def rec(name, status, secs, detail=""):
    with LOCK:
        SAMPLES.append((name, status, secs))
        if status == 0 or status >= 500 or status == 429:
            ERRORS.append((time.strftime("%H:%M:%S"), name, status, str(detail)[:200]))


def totp(secret, t=None):
    """RFC 6238 SHA1/6 digits. Keycloak's hidden `totpSecret` is the raw key text."""
    h = hmac.new(secret.encode(), struct.pack(">Q", int((t or time.time()) // 30)), hashlib.sha1).digest()
    o = h[-1] & 15
    return "%06d" % ((struct.unpack(">I", h[o:o + 4])[0] & 0x7FFFFFFF) % 1000000)


# ── HTTP ───────────────────────────────────────────────────────────────────
class Conn(threading.local):
    c = None


_conn = Conn()


def call(method, path, token=None, body=None, headers=None, name=None, raw=None, timeout=30, base="/api/v1"):
    """One request over a per-thread keep-alive connection. Returns (status, json|bytes)."""
    h = dict(headers or {})
    if token:
        h["Authorization"] = "Bearer " + token
    data = raw
    if body is not None:
        data = json.dumps(body).encode()
        h["Content-Type"] = "application/json"
    t0 = time.time()
    status, out = 0, b""
    for attempt in (0, 1):
        try:
            if _conn.c is None:
                _conn.c = http.client.HTTPSConnection(HOST, 443, context=CTX, timeout=timeout)
            _conn.c.request(method, base + path, body=data, headers=h)
            r = _conn.c.getresponse()
            out = r.read()
            status = r.status
            break
        except Exception as e:  # stale keep-alive: reconnect once
            try:
                _conn.c.close()
            except Exception:
                pass
            _conn.c = None
            if attempt:
                out = repr(e).encode()
    secs = time.time() - t0
    nm = name or (method + " " + re.sub(r"[0-9a-f]{8}-[0-9a-f-]{27}", ":id", path.split("?")[0]))
    try:
        js = json.loads(out) if out[:1] in (b"{", b"[") else out
    except Exception:
        js = out
    rec(nm, status, secs, js if status >= 400 or status == 0 else "")
    return status, js


# ── Keycloak ───────────────────────────────────────────────────────────────
def kc_token(realm, form):
    u = f"https://{HOST}/auth/realms/{realm}/protocol/openid-connect/token"
    req = urllib.request.Request(u, data=urllib.parse.urlencode(form).encode())
    with urllib.request.urlopen(req, context=CTX, timeout=30) as r:
        return json.load(r)


class NoRedir(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *a, **k):
        return None


LAST_STEP = {}


def web_login(realm, cred):
    """Authorization code + PKCE through the real login page (password, then OTP).
    Keycloak refuses to accept a TOTP code twice (otpPolicyCodeReusable=false), so a second
    login of the same user within the same 30 s window waits for the next window; a rejected
    code is retried once, never in a tight loop (two quick failures lock the user for 60 s)."""
    for attempt in (0, 1):
        try:
            return _web_login(realm, cred)
        except OtpRejected:
            if attempt:
                raise RuntimeError("OTP rejected twice for " + cred["username"] + " (clock skew, or lock-out: wait 60 s)")
            time.sleep(31)


class OtpRejected(Exception):
    pass


def _web_login(realm, cred):
    jar = http.cookiejar.CookieJar()
    op = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar), urllib.request.HTTPSHandler(context=CTX), NoRedir)

    def get(u, d=None):
        try:
            x = op.open(urllib.request.Request(u, data=urllib.parse.urlencode(d).encode() if d else None), timeout=30)
            return x.read().decode(), None
        except urllib.error.HTTPError as e:
            if e.code in (301, 302, 303):
                return "", e.headers["Location"]
            raise

    ver = secrets.token_hex(32)
    chal = base64.urlsafe_b64encode(hashlib.sha256(ver.encode()).digest()).rstrip(b"=").decode()
    q = urllib.parse.urlencode({"client_id": "securepatrol-web", "response_type": "code", "scope": "openid",
                                "redirect_uri": f"https://{HOST}/", "code_challenge": chal,
                                "code_challenge_method": "S256", "state": "s", "nonce": "n"})
    page, _ = get(f"https://{HOST}/auth/realms/{realm}/protocol/openid-connect/auth?{q}")
    form = {"username": cred["username"], "password": cred["password"]}
    for _ in range(4):
        m = re.search(r'action="([^"]*(?:authenticate|required-action)[^"]*)"', page)
        if not m:
            raise RuntimeError("login page without form: " + (re.findall("<title>([^<]*)", page) or ["?"])[0])
        page, loc = get(html.unescape(m.group(1)), form)
        while loc and f"/auth/realms/{realm}/" in loc:
            page, loc = get(loc)
        if loc:
            break
        if 'name="otp"' in page:
            if "otp" in form:
                raise OtpRejected()
            step = int(time.time() // 30)
            if LAST_STEP.get(cred["totp"]) == step:
                time.sleep(30 - time.time() % 30 + 1)
            LAST_STEP[cred["totp"]] = int(time.time() // 30)
            form = {"otp": totp(cred["totp"]), "login": "Sign In"}
        else:
            raise RuntimeError("unexpected login step: " + (re.findall("<title>([^<]*)", page) or ["?"])[0])
    code = re.search(r"[?&]code=([^&]+)", loc or "")
    if not code:
        raise RuntimeError("no code in redirect " + str(loc))
    return kc_token(realm, {"grant_type": "authorization_code", "client_id": "securepatrol-web",
                            "code": code.group(1), "redirect_uri": f"https://{HOST}/", "code_verifier": ver})


class Session:
    """Access token with automatic refresh (Keycloak access tokens last 15 min)."""

    def __init__(self, realm, client, tok):
        self.realm, self.client, self.tok = realm, client, tok
        self.exp = time.time() + tok["expires_in"]
        self.lock = threading.Lock()

    def token(self):
        with self.lock:
            if time.time() > self.exp - 120:
                self.tok = kc_token(self.realm, {"grant_type": "refresh_token", "client_id": self.client,
                                                 "refresh_token": self.tok["refresh_token"]})
                self.exp = time.time() + self.tok["expires_in"]
            return self.tok["access_token"]


# ── setup ──────────────────────────────────────────────────────────────────
SITES = [("securepatrol-client1", "Load Site A"), ("securepatrol-client2", "Load Site B")]
GUARDS_PER_SITE = 5
CHECKPOINTS = 10
PINS = ["482915", "739206", "615038", "270481", "904763"]


def must(r, ok=(200, 201, 204), what=""):
    if r[0] not in ok:
        raise RuntimeError(f"{what}: HTTP {r[0]} {r[1]}")
    return r[1]


def setup(creds, state_path):
    state = {"sites": []}
    for realm, sname in SITES:
        adm = Session(realm, "securepatrol-web", web_login(realm, creds[realm]["admin"]))
        t = adm.token()
        site = ([x for x in must(call("GET", "/sites", t), what="sites") if x["name"] == sname] or [None])[0] \
            or must(call("POST", "/sites", t, {"name": sname, "timezone": "Asia/Jakarta", "address": "Load test"}), what="site")
        sid = site["id"]
        have = {x["employee_id"]: x for x in must(call("GET", f"/guards?site_id={sid}", t), what="guards")}
        guards = []
        for i in range(GUARDS_PER_SITE):
            eid = f"LT{sname[-1]}{i + 1:02d}"
            g = have.get(eid) or must(call("POST", "/guards", t, {"name": f"LT {sname[-1]}{i + 1}", "employee_id": eid,
                                                                   "site_id": sid, "role": "guard", "status": "active"}), what="guard")
            must(call("PUT", f"/guards/{g['id']}/pin", t, {"pin": PINS[i]}), what="pin")
            guards.append({"id": g["id"], "name": g["name"], "pin": PINS[i]})
        cps = [{"name": f"CP {n + 1}", "order_index": n, "sla_minutes": None} for n in range(CHECKPOINTS)]
        route = ([x for x in must(call("GET", f"/patrol/routes?site_id={sid}", t), what="routes") if x["name"] == "Perimeter"] or [None])[0] \
            or must(call("POST", "/patrol/routes", t, {"site_id": sid, "name": "Perimeter", "checkpoints": cps}), what="route")
        now = time.time()
        iso = lambda x: time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(x))
        for g in guards:  # one-off shifts: a running one for the whole run + UI pass (re-runs add another; harmless)
            must(call("POST", "/shifts", t, {"site_id": sid, "guard_id": g["id"], "start_time": iso(now - 1800),
                                              "end_time": iso(now + 9 * 3600)}), what="shift", ok=(200, 201, 409))
        acc = must(call("POST", "/device-accounts", t, {"name": f"load tablet {sname[-1]}"}), what="device account")
        tab = kc_token(realm, {"grant_type": "password", "client_id": "securepatrol-tablet", "username": acc["account"]["username"],
                               "password": acc["password"], "scope": "openid offline_access"})
        tt = tab["access_token"]
        must(call("POST", "/device/register", tt, {"name": f"Load tablet {sname[-1]}"}), what="register")
        devs = must(call("GET", "/devices", t), what="devices")
        dev = [d for d in devs if d["name"] == f"Load tablet {sname[-1]}"][0]
        must(call("PUT", f"/devices/{dev['id']}", t, {"name": dev["name"], "site_id": sid, "status": "active"}), what="approve")
        state["sites"].append({"realm": realm, "site_id": sid, "name": sname, "route_id": route["id"], "guards": guards,
                               "tablet": {"refresh_token": tab["refresh_token"]}})
        print(f"{realm}: site {sid}, {len(guards)} guards, route {route['id']}, tablet approved")
    json.dump(state, open(state_path, "w"), indent=1)
    os.chmod(state_path, 0o600)


# ── run ────────────────────────────────────────────────────────────────────
class Stop:
    def __init__(self, deadline):
        self.deadline = deadline

    def __bool__(self):
        return time.time() >= self.deadline

    def sleep(self, s):
        time.sleep(min(s, max(0, self.deadline - time.time())))


def iso(ts):
    return time.strftime("%Y-%m-%dT%H:%M:%S", time.gmtime(ts)) + ".%03dZ" % (int(ts * 1000) % 1000)


class Guard:
    def __init__(self, site, g, tablet, route, stop, args, barrier_evt):
        self.site, self.g, self.tablet, self.route, self.stop, self.a = site, g, tablet, route, stop, args
        self.codes = [c["qr_code"] for c in route["checkpoints"]]
        self.cp_ids = [c["id"] for c in route["checkpoints"]]
        self.gtok, self.gtok_at = None, 0
        self.evt = barrier_evt
        self.n = 0

    def hdr(self):
        if not self.gtok or time.time() - self.gtok_at > 3600:
            r = call("POST", "/device/guard-sessions", self.tablet.token(), {"guard_id": self.g["id"], "pin": self.g["pin"]})
            if r[0] == 200:
                self.gtok, self.gtok_at = r[1]["token"], time.time()
        h = {"X-Guard-ID": self.g["id"]}
        if self.gtok:
            h["X-Guard-Token"] = self.gtok
        return h

    def post(self, path, body, **kw):
        return call("POST", path, self.tablet.token(), body, headers=self.hdr(), **kw)

    def photo(self, kind):
        pid = str(uuid.uuid4())
        boundary = "----lt" + secrets.token_hex(8)
        parts = (f'--{boundary}\r\nContent-Disposition: form-data; name="kind"\r\n\r\n{kind}\r\n'
                 f'--{boundary}\r\nContent-Disposition: form-data; name="id"\r\n\r\n{pid}\r\n'
                 f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="p.jpg"\r\nContent-Type: image/jpeg\r\n\r\n').encode() \
            + JPEG + f"\r\n--{boundary}--\r\n".encode()
        h = self.hdr()
        h["Content-Type"] = "multipart/form-data; boundary=" + boundary
        r = call("POST", "/media", self.tablet.token(), headers=h, raw=parts)
        return r[1].get("object_key") if r[0] in (200, 201) and isinstance(r[1], dict) else None

    def clock_in(self):
        r = self.post("/attendance/clock-in", {"id": str(uuid.uuid4()), "clock_in_at": iso(time.time())})
        if r[0] not in (200, 201, 409):
            ERRORS.append((time.strftime("%H:%M:%S"), "clock-in", r[0], str(r[1])[:200]))

    def incident_and_visitor(self):
        key = self.photo("incident")
        self.post("/incidents", {"id": str(uuid.uuid4()), "type": "Suspicious person", "severity": random.choice(["low", "medium", "high"]),
                                 "description": "load test", "location": "Gate", "photo_object_keys": [key] if key else [],
                                 "occurred_at": iso(time.time())})
        vid = str(uuid.uuid4())
        self.post("/visitors", {"id": vid, "visitor_name": "Load Visitor", "id_type": "KTP", "id_number": "3201000000000001",
                                "plate": "B1234LT", "purpose": "Delivery", "entry_at": iso(time.time())})
        self.post(f"/visitors/{vid}/exit", {"exit_at": iso(time.time())}, name="POST /visitors/:id/exit")
        self.post("/defects", {"id": str(uuid.uuid4()), "defect_type": "Lamp", "location": "Gate", "description": "load test",
                               "reported_at": iso(time.time())})

    def patrol(self):
        sid = str(uuid.uuid4())
        self.post("/patrol/sessions", {"id": sid, "route_id": self.route["id"], "started_at": iso(time.time())})
        for i, code in enumerate(self.codes):
            self.stop.sleep(self.a.scan_every * random.uniform(0.7, 1.3))
            if self.stop:
                return
            self.maybe_sync()
            self.post(f"/patrol/sessions/{sid}/scans", {"id": str(uuid.uuid4()), "qr_payload": code, "method": "qr",
                                                        "scanned_at": iso(time.time())}, name="POST /patrol/sessions/:id/scans")
            if i % 3 == 0:
                call("GET", f"/patrol/sessions/{sid}", self.tablet.token(), headers=self.hdr(), name="GET /patrol/sessions/:id")
        self.post(f"/patrol/sessions/{sid}/end", {"ended_at": iso(time.time())}, name="POST /patrol/sessions/:id/end")

    def sync_queue(self):
        """Replay what a tablet queued while offline: session, 10 scans, end, incident+photo, visitor+exit."""
        t0 = time.time()
        base = t0 - 1500
        st = []
        sid = str(uuid.uuid4())
        st.append(self.post("/patrol/sessions", {"id": sid, "route_id": self.route["id"], "started_at": iso(base)}, name="sync POST sessions")[0])
        for i, code in enumerate(self.codes):
            st.append(self.post(f"/patrol/sessions/{sid}/scans", {"id": str(uuid.uuid4()), "qr_payload": code, "method": "qr",
                                                                  "scanned_at": iso(base + 60 * (i + 1))}, name="sync POST scans")[0])
        st.append(self.post(f"/patrol/sessions/{sid}/end", {"ended_at": iso(base + 700)}, name="sync POST end")[0])
        key = self.photo("incident")
        st.append(self.post("/incidents", {"id": str(uuid.uuid4()), "type": "Fire alarm", "severity": "medium", "description": "queued offline",
                                           "photo_object_keys": [key] if key else [], "occurred_at": iso(base + 800)}, name="sync POST incidents")[0])
        vid = str(uuid.uuid4())
        st.append(self.post("/visitors", {"id": vid, "visitor_name": "Queued Visitor", "entry_at": iso(base + 900)}, name="sync POST visitors")[0])
        st.append(self.post(f"/visitors/{vid}/exit", {"exit_at": iso(base + 1000)}, name="sync POST visitor exit")[0])
        dt = time.time() - t0
        ok = all(s in (200, 201) for s in st)
        with LOCK:
            EVENTS["sync"].append({"guard": self.g["name"], "seconds": round(dt, 2), "requests": len(st), "ok": ok})
        # idempotent replay of the session create must answer 200 with the stored row
        r = self.post("/patrol/sessions", {"id": sid, "route_id": self.route["id"], "started_at": iso(base)}, name="sync replay sessions")
        if r[0] != 200:
            ERRORS.append((time.strftime("%H:%M:%S"), "sync replay", r[0], "expected 200 for a known id"))

    def maybe_sync(self):
        if self.evt.round != self.round:
            self.round = self.evt.round
            self.evt.wait_barrier(self.round)
            self.sync_queue()

    def run(self):
        self.round = 0
        self.stop.sleep(random.uniform(0, 5))
        self.hdr()
        self.clock_in()
        while not self.stop:
            self.maybe_sync()
            self.n += 1
            call("GET", "/device/status", self.tablet.token(), headers=self.hdr(), name="GET /device/status")
            self.patrol()
            if self.n % 2 == 0:
                self.incident_and_visitor()
            self.stop.sleep(random.uniform(5, 15))


class SyncTrigger:
    """Every N minutes all guards replay their queue at the same moment (barrier per round)."""

    def __init__(self, parties):
        self.round, self.parties, self._bars = 0, parties, {}

    def start_round(self):
        with LOCK:
            self._bars[self.round + 1] = threading.Barrier(self.parties, timeout=90)
            self.round += 1

    def wait_barrier(self, rnd):
        try:
            self._bars[rnd].wait()
        except threading.BrokenBarrierError:
            pass


def web_user(realm, role, sess, site_id, stop, args):
    """A dashboard user: page-load burst (parallel, like the SPA) + polling + occasional reports."""
    me_endpoints = {
        "admin": ["/sites", f"/dashboard/summary?site_id={site_id}", "/sos/active", f"/patrol/live?site_id={site_id}",
                  f"/attendance/daily?site_id={site_id}&date={time.strftime('%Y-%m-%d')}", f"/incidents?site_id={site_id}&limit=50"],
        "supervisor": ["/sites", f"/dashboard/summary?site_id={site_id}", "/sos/active", f"/patrol/live?site_id={site_id}",
                       f"/attendance/daily?site_id={site_id}&date={time.strftime('%Y-%m-%d')}", f"/incidents?site_id={site_id}&limit=50"],
        "hr": ["/sites", f"/dashboard/summary?site_id={site_id}", f"/attendance/daily?site_id={site_id}&date={time.strftime('%Y-%m-%d')}",
               f"/guards?site_id={site_id}"],
        "client": ["/sites", f"/dashboard/summary?site_id={site_id}", f"/patrol/live?site_id={site_id}", f"/incidents?site_id={site_id}&limit=50"],
    }[role]
    month = time.strftime("%Y-%m")
    day = time.strftime("%Y-%m-%d")
    reports = {
        "admin": [f"/reports/attendance/monthly?site_id={site_id}&month={month}", f"/reports/patrol/sessions?site_id={site_id}&from={day}&to={day}",
                  f"/reports/patrol-compliance?site_id={site_id}&from={day}&to={day}", f"/reports/guards/kpi?site_id={site_id}&from={day}&to={day}",
                  f"/reports/attendance/monthly?site_id={site_id}&month={month}&format=xlsx"],
        "supervisor": [f"/reports/patrol/sessions?site_id={site_id}&from={day}&to={day}", f"/sos?site_id={site_id}", f"/defects?site_id={site_id}"],
        "hr": [f"/reports/attendance/monthly?site_id={site_id}&month={month}", f"/reports/guards/kpi?site_id={site_id}&from={day}&to={day}"],
        "client": [f"/reports/patrol/sessions?site_id={site_id}&from={day}&to={day}"],
    }[role]
    pool = ThreadPoolExecutor(max_workers=6)
    stop.sleep(random.uniform(0, args.poll_every))
    n = 0
    while not stop:
        n += 1
        t0 = time.time()
        tok = sess.token()
        res = list(pool.map(lambda p: call("GET", p, tok, name="page " + re.sub(r"\?.*", "", re.sub(r"[0-9a-f]{8}-[0-9a-f-]{27}", ":id", p))), me_endpoints))
        dt = time.time() - t0
        with LOCK:
            EVENTS["dashboard"].append({"user": f"{role}@{realm[-1]}", "seconds": round(dt, 3), "ok": all(r[0] == 200 for r in res)})
        if n % 6 == 0:
            for p in reports:
                call("GET", p, sess.token(), name="report " + re.sub(r"\?.*", "", p).split("/", 2)[-1].replace(site_id, ":site"))
        stop.sleep(args.poll_every)


def tablet_beacon(site, tablet, stop):
    n = 0
    while not stop:
        call("POST", "/device/telemetry", tablet.token(), {"app_version": "loadtest", "app_build": "1", "platform": "android", "os_version": "14",
                                                           "outbox_pending": 0, "outbox_failed": 0, "sos_pending": 0, "clock_offset_ms": 0,
                                                           "private_ca": True, "errors": []})
        if n % 5 == 0:
            call("GET", "/device/routes", tablet.token())
            call("GET", "/device/roster", tablet.token())
            call("GET", "/device/me", tablet.token())
        n += 1
        stop.sleep(61)


def sampler(stop, report_dir):
    names = ["api", "notification-worker", "postgres", "redis", "keycloak", "nginx", "minio"]
    while not stop:
        try:
            out = subprocess.run(["docker", "stats", "--no-stream", "--format", "{{.Name}}|{{.CPUPerc}}|{{.MemUsage}}"],
                                 capture_output=True, text=True, timeout=60).stdout
            row = {"t": time.strftime("%H:%M:%S")}
            for ln in out.splitlines():
                nm, cpu, mem = ln.split("|")
                short = nm.replace("infra-", "").rsplit("-", 1)[0]
                if short in names:
                    row[short] = {"cpu": float(cpu.strip("%")), "mem": mem.split("/")[0].strip()}
            with LOCK:
                EVENTS["docker"].append(row)
        except Exception:
            pass
        stop.sleep(60)


def pct(v, p):
    v = sorted(v)
    return v[min(len(v) - 1, int(len(v) * p))] if v else 0


def report(args, started, ended, log_errors):
    with LOCK:
        samples, errors, ev = list(SAMPLES), list(ERRORS), {k: list(v) for k, v in EVENTS.items()}
    by = {}
    for n, s, d in samples:
        by.setdefault(n, []).append((s, d))
    rows = []
    for n, v in sorted(by.items()):
        d = [x[1] * 1000 for x in v]
        st = {}
        for s, _ in v:
            st[s] = st.get(s, 0) + 1
        rows.append({"endpoint": n, "n": len(v), "p50": round(pct(d, .5)), "p95": round(pct(d, .95)), "p99": round(pct(d, .99)),
                     "max": round(max(d)), "status": st})
    sy = [e["seconds"] for e in ev["sync"]]
    db = [e["seconds"] for e in ev["dashboard"]]
    bad = [e for e in errors]
    res = {"started": started, "ended": ended, "requests": len(samples), "errors": len(bad), "error_samples": bad[:40],
           "sync": {"queues": len(sy), "p50": pct(sy, .5), "p95": pct(sy, .95), "max": max(sy or [0]), "failed_queues": sum(1 for e in ev["sync"] if not e["ok"])},
           "dashboard": {"loads": len(db), "p50": pct(db, .5), "p95": pct(db, .95), "max": max(db or [0]), "failed": sum(1 for e in ev["dashboard"] if not e["ok"])},
           "endpoints": rows, "docker": ev["docker"], "api_log_errors": log_errors,
           "pass": {"dashboard_lt_2s_p95": pct(db, .95) < 2, "sync_lt_10s_max": max(sy or [0]) < 10, "no_errors": not bad and not log_errors}}
    os.makedirs(args.report, exist_ok=True)
    json.dump(res, open(os.path.join(args.report, "report.json"), "w"), indent=1)
    md = [f"# Load test {started} -> {ended}", "", f"Requests: {len(samples)}; errors (5xx/429/timeouts/unexpected): {len(bad)}; API log error lines: {len(log_errors)}", "",
          f"Dashboard page set (parallel, {len(db)} loads): p50 {res['dashboard']['p50']}s, p95 {res['dashboard']['p95']}s, max {res['dashboard']['max']}s, failed {res['dashboard']['failed']}",
          f"Sync queue ({len(sy)} guard queues of 16 requests): p50 {res['sync']['p50']}s, p95 {res['sync']['p95']}s, max {res['sync']['max']}s, failed {res['sync']['failed_queues']}", "",
          "| endpoint | n | p50 ms | p95 ms | p99 ms | max ms | status |", "|---|---|---|---|---|---|---|"]
    for r in rows:
        md.append(f"| {r['endpoint']} | {r['n']} | {r['p50']} | {r['p95']} | {r['p99']} | {r['max']} | {r['status']} |")
    open(os.path.join(args.report, "report.md"), "w").write("\n".join(md) + "\n")
    print("\n".join(md[:8]))
    print("pass:", res["pass"])
    return res


def run(creds, state, args):
    started = time.strftime("%Y-%m-%d %H:%M:%S")
    t_start = time.time()
    stop = Stop(t_start + args.minutes * 60)
    threads, parties = [], 0
    sync = SyncTrigger(len(state["sites"]) * GUARDS_PER_SITE)
    web_sessions = []
    for s in state["sites"]:
        realm = s["realm"]
        tablet = Session(realm, "securepatrol-tablet", kc_token(realm, {"grant_type": "refresh_token", "client_id": "securepatrol-tablet",
                                                                          "refresh_token": s["tablet"]["refresh_token"]}))
        adm = Session(realm, "securepatrol-web", web_login(realm, creds[realm]["admin"]))
        routes = call("GET", f"/patrol/routes?site_id={s['site_id']}", adm.token())[1]
        route = [r for r in routes if r["id"] == s["route_id"]][0]
        tr = call("GET", "/device/routes", tablet.token())[1]
        route["checkpoints"] = [dict(c, qr_code=[x for x in route["checkpoints"] if x["id"] == c["id"]][0]["qr_code"]) for c in tr[0]["checkpoints"]]
        for g in s["guards"]:
            threads.append(threading.Thread(target=Guard(s, g, tablet, route, stop, args, sync).run, daemon=True))
        threads.append(threading.Thread(target=tablet_beacon, args=(s, tablet, stop), daemon=True))
        web_sessions.append((realm, "admin", adm, s["site_id"]))
        for role in ("supervisor", "hr", "client"):
            web_sessions.append((realm, role, Session(realm, "securepatrol-web", web_login(realm, creds[realm][role])), s["site_id"]))
            time.sleep(1)
    for realm, role, sess, sid in web_sessions:
        threads.append(threading.Thread(target=web_user, args=(realm, role, sess, sid, stop, args), daemon=True))
    threads.append(threading.Thread(target=sampler, args=(stop, args.report), daemon=True))
    for t in threads:
        t.start()
    print(f"started {len(threads)} threads, {args.minutes} min", flush=True)
    nxt = t_start + args.first_sync * 60
    while not stop:
        if time.time() >= nxt:
            sync.start_round()
            nxt += args.sync_every * 60
        time.sleep(1)
        if int(time.time() - t_start) % 300 == 0:
            with LOCK:
                print(f"[{time.strftime('%H:%M:%S')}] requests={len(SAMPLES)} errors={len(ERRORS)} syncs={len(EVENTS['sync'])}", flush=True)
            time.sleep(1)
    time.sleep(5)
    since = int(time.time() - t_start) + 120
    logs = subprocess.run(["docker", "compose", "logs", "--since", f"{since}s", "api", "notification-worker"], cwd=os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."),
                          capture_output=True, text=True).stdout
    log_errors = [l[:300] for l in logs.splitlines() if re.search(r'"level":"(error|fatal|panic)"|\bpanic\b', l)][:50]
    report(args, started, time.strftime("%Y-%m-%d %H:%M:%S"), log_errors)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("cmd", choices=["setup", "run"])
    ap.add_argument("--creds", required=True)
    ap.add_argument("--state", required=True)
    ap.add_argument("--minutes", type=float, default=60)
    ap.add_argument("--scan-every", type=float, default=20, help="seconds between scans of one guard")
    ap.add_argument("--poll-every", type=float, default=10, help="seconds between dashboard reloads of one web user")
    ap.add_argument("--sync-every", type=float, default=10, help="minutes between offline-queue replays")
    ap.add_argument("--first-sync", type=float, default=2, help="minutes until the first replay")
    ap.add_argument("--report", default="loadtest-report")
    a = ap.parse_args()
    creds = json.load(open(a.creds))
    if a.cmd == "setup":
        setup(creds, a.state)
    else:
        run(creds, json.load(open(a.state)), a)


if __name__ == "__main__":
    main()
