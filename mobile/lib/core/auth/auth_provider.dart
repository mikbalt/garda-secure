import 'dart:async';
import 'dart:convert';

import 'package:flutter/widgets.dart';

import '../../config/env.dart';
import '../../shared/models/models.dart';
import '../api/api_client.dart';
import '../database/local_db.dart';
import '../services/clock.dart';
import '../services/photo_service.dart';
import '../services/pin_verifier.dart';
import '../storage/secure_store.dart';
import 'guard_sessions.dart';
import 'keycloak_service.dart';

/// Where the tablet is in its life cycle.
enum Phase { booting, setup, login, pending, revoked, locked, unlocked }

/// Outcome of a PIN entry on the lock screen.
enum UnlockStatus { unlocked, wrongPin, locked, notAllowed, pinNotSet, rosterStale }

class UnlockResult {
  const UnlockResult(this.status, {this.attemptsLeft = 0, this.waitSeconds = 0});

  final UnlockStatus status;
  final int attemptsLeft;
  final int waitSeconds;
}

/// Sign-out refused: unsent items are never dropped silently (MOB-2).
class OutboxNotEmpty implements Exception {
  const OutboxNotEmpty(this.count);
  final int count;
}

/// The tablet's session: server config, device registration (D-001), site
/// reference data cached for offline use, and which guard is using it.
class DeviceSession extends ChangeNotifier with WidgetsBindingObserver {
  DeviceSession({required this.store, required this.auth, required this.db, PinAttempts? pinAttempts, ApiClient? api})
      : pinAttempts = pinAttempts ?? PinAttempts(),
        _apiOverride = api {
    this.pinAttempts.onChanged = (state) => unawaited(db.put(CacheKeys.pinLockout, state));
  }

  final ApiClient? _apiOverride;

  final SecureStore store;
  final KeycloakService auth;
  final LocalDb db;

  late final ApiClient api = _apiOverride ?? ApiClient(auth, () => config!);
  late final GuardSessions guardSessions = GuardSessions(api, store);

  /// Persistent, escalating PIN lockout (MOB-6).
  final PinAttempts pinAttempts;

  Phase phase = Phase.booting;
  ServerConfig? config;
  Device? device;
  Site? site;
  SiteConfig siteConfig = const SiteConfig();
  List<RosterGuard> roster = const [];
  List<PatrolRoute> routes = const [];
  List<SosAlert> sosAlerts = const [];
  RosterGuard? guard;
  GuardStatus? guardStatus;
  DateTime? referenceSyncedAt;
  Locale locale = const Locale('id');

  /// The acting guard's PIN was confirmed by the server (D-035); false after
  /// an offline unlock.
  bool guardVerified = false;

  /// Why the tablet was locked by the server after an offline-looking unlock
  /// (shown once on the lock screen): `{code, name}`.
  Map<String, String>? lockNotice;

  /// Set by the patrol feature so auto-lock is relaxed during a running patrol.
  bool patrolRunning = false;

  /// Lifted the outbox pause (wired in main: the device is active again).
  VoidCallback? onDeviceActive;

  /// Push hooks (PushService.attach): push is configured, allowed and
  /// registered; the tablet registered itself; an admin approved it; the
  /// device is about to sign out (still authenticated).
  bool Function()? pushHealthy;
  VoidCallback? onRegistered;
  VoidCallback? onApproved;
  Future<void> Function()? beforeSignOut;

  /// SOS of other tablets is polled every 15 s while someone works on the
  /// tablet and every 30 s on the lock screen (MOB-11). With healthy push it
  /// relaxes to [sosPollPush] — never slower: SOS never relies on push alone.
  static const sosPollUnlocked = Duration(seconds: 15);
  static const sosPollLocked = Duration(seconds: 30);
  static const sosPollPush = Duration(seconds: 60);

  /// No unlock without a roster sync in this period (MOB-6). SOS still works.
  static const maxOfflineAge = Duration(days: 7);

  /// Background longer than this locks the tablet (MOB-11).
  static const backgroundLock = Duration(seconds: 30);

  /// How long the lock screen waits for the server's PIN check.
  Duration serverCheckWait = const Duration(seconds: 3);

  DateTime _lastActivity = DateTime.now();
  DateTime? _backgroundAt;
  Timer? _ticker;
  int _tick = 0;

  // ── boot & setup ───────────────────────────────────────────────────────────

  Future<void> boot() async {
    WidgetsBinding.instance.addObserver(this);
    final lang = await store.read(SecureKeys.language);
    if (lang == 'en') locale = const Locale('en');
    final raw = await store.read(SecureKeys.config);
    config = raw == null ? null : ServerConfig.fromJson((jsonDecode(raw) as Map).cast<String, dynamic>());
    await _loadLocalState();
    if (config == null) return _go(Phase.setup);
    if (!await auth.hasSession) return _go(Phase.login);

    await _loadCache();
    _go(device?.isActive == true ? Phase.locked : (device?.status == DeviceStatusKind.revoked ? Phase.revoked : Phase.pending));
    _startTicker();
    unawaited(refreshDevice());
  }

  /// Clock offset and PIN lockouts survive restarts.
  Future<void> _loadLocalState() async {
    final off = await db.get(CacheKeys.clockOffset);
    if (off is num) AppClock.restore(Duration(milliseconds: off.toInt()));
    AppClock.onChanged = (d) => unawaited(db.put(CacheKeys.clockOffset, d.inMilliseconds));
    pinAttempts.load(await db.getMap(CacheKeys.pinLockout));
  }

  Future<void> saveServer(ServerConfig cfg) async {
    config = cfg;
    await store.write(SecureKeys.config, jsonEncode(cfg.toJson()));
    _go(Phase.login);
  }

  /// Back from login to the server setup screen (the UI asks for approval
  /// and refuses while items are queued, MOB-2).
  Future<void> editServer() async {
    final n = await db.outboxCount();
    if (n > 0) throw OutboxNotEmpty(n);
    _go(Phase.setup);
  }

  Future<void> setLanguage(String code) async {
    locale = Locale(code);
    await store.write(SecureKeys.language, code);
    notifyListeners();
  }

  /// Signs in with the device account and registers the tablet.
  Future<void> login(String username, String password, String deviceName) async {
    await auth.login(config!, username, password);
    final res = await api.post('/device/register', body: {'name': deviceName.trim()});
    device = Device.fromJson((res as Map).cast<String, dynamic>());
    await db.put(CacheKeys.device, device!.toJson());
    onRegistered?.call();
    _startTicker();
    await refreshDevice();
    if (phase == Phase.login) _go(Phase.pending);
  }

  // ── device state ───────────────────────────────────────────────────────────

  /// Re-reads the device record; moves between pending/active/revoked.
  Future<void> refreshDevice() async {
    try {
      final res = (await api.get('/device/me') as Map).cast<String, dynamic>();
      device = Device.fromJson((res['device'] as Map).cast<String, dynamic>());
      await db.put(CacheKeys.device, device!.toJson());
      if (res['site'] is Map) {
        site = Site.fromJson((res['site'] as Map).cast<String, dynamic>());
        AppClock.setSiteZone(site!.timezone);
        await db.put(CacheKeys.site, site!.toJson());
      }
      if (res['config'] is Map) {
        siteConfig = SiteConfig.fromJson((res['config'] as Map).cast<String, dynamic>());
        await db.put(CacheKeys.siteConfig, siteConfig.toJson());
      }
      if (device!.status == DeviceStatusKind.revoked) {
        await _onRevoked();
      } else if (!device!.isActive) {
        _go(Phase.pending);
      } else {
        onDeviceActive?.call();
        if (phase == Phase.pending || phase == Phase.revoked || phase == Phase.login || phase == Phase.booting) {
          await syncReference();
          _go(Phase.locked);
          onApproved?.call();
        }
      }
    } on ApiException catch (e) {
      if (e.code == 'auth_expired') return expire();
      if (e.code == 'device_revoked') return _onRevoked();
      if (e.code == 'device_unregistered' && device != null) {
        // Registration was deleted on the server: register again.
        try {
          await api.post('/device/register', body: {'name': device!.name});
          onRegistered?.call();
        } catch (_) {}
      }
      // Offline: keep using the cache.
    }
  }

  /// The server refused the device (outbox paused): find out why and show
  /// the right screen.
  void onDeviceDenied(String code) => unawaited(refreshDevice());

  /// Revoked (lost/stolen tablet): drop everything that lets someone use it
  /// offline — roster with PIN hashes, routes, guard state and tokens. Unsent
  /// items stay (they are the only copy) until sent or discarded.
  Future<void> _onRevoked() async {
    guard = null;
    guardStatus = null;
    guardVerified = false;
    roster = const [];
    routes = const [];
    sosAlerts = const [];
    await db.put(CacheKeys.roster, null);
    await db.put(CacheKeys.routes, null);
    await db.put(CacheKeys.rosterSyncedAt, null);
    await db.deletePrefix('guard_status:');
    await db.deletePrefix('patrol:');
    await db.deletePrefix('clock:');
    await db.put(CacheKeys.visitors, null);
    await guardSessions.clearAll();
    _go(Phase.revoked);
  }

  /// Roster (with PIN hashes) and routes, cached for offline use.
  Future<void> syncReference() async {
    try {
      final r = (await api.get('/device/roster') as Map).cast<String, dynamic>();
      roster = ((r['guards'] as List?) ?? const []).map((e) => RosterGuard.fromJson((e as Map).cast<String, dynamic>())).toList()
        ..sort((a, b) => a.name.toLowerCase().compareTo(b.name.toLowerCase()));
      await db.put(CacheKeys.roster, roster.map((g) => g.toJson()).toList());
      final rt = await api.get('/device/routes');
      routes = ((rt as List?) ?? const []).map((e) => PatrolRoute.fromJson((e as Map).cast<String, dynamic>())).toList();
      await db.put(CacheKeys.routes, routes.map((r) => r.toJson()).toList());
      referenceSyncedAt = AppClock.now();
      await db.put(CacheKeys.rosterSyncedAt, referenceSyncedAt!.toIso8601String());
      // A guard removed or deactivated on the server loses access right away.
      if (guard != null && !roster.any((g) => g.id == guard!.id)) lock();
      notifyListeners();
    } on ApiException catch (e) {
      if (e.code == 'auth_expired') return expire();
      if (e.status == 403 && (e.code ?? '').startsWith('device_')) await refreshDevice();
    }
  }

  Future<void> _loadCache() async {
    final d = await db.getMap(CacheKeys.device);
    if (d != null) device = Device.fromJson(d);
    final s = await db.getMap(CacheKeys.site);
    if (s != null) {
      site = Site.fromJson(s);
      AppClock.setSiteZone(site!.timezone);
    }
    final c = await db.getMap(CacheKeys.siteConfig);
    if (c != null) siteConfig = SiteConfig.fromJson(c);
    roster = (await db.getList(CacheKeys.roster)).map(RosterGuard.fromJson).toList();
    routes = (await db.getList(CacheKeys.routes)).map(PatrolRoute.fromJson).toList();
    final synced = await db.get(CacheKeys.rosterSyncedAt);
    referenceSyncedAt = synced is String ? DateTime.tryParse(synced) : null;
    if (referenceSyncedAt == null && roster.isNotEmpty) {
      // Upgraded from a version that did not record it: start counting now.
      referenceSyncedAt = AppClock.now();
      await db.put(CacheKeys.rosterSyncedAt, referenceSyncedAt!.toIso8601String());
    }
  }

  /// The cached roster is too old to trust for offline unlocks (MOB-6).
  bool get rosterStale => referenceSyncedAt != null && AppClock.now().difference(referenceSyncedAt!) > maxOfflineAge;

  // ── guard lock / unlock ────────────────────────────────────────────────────

  /// Checks [pin] for [g]: locally against the synced bcrypt hash (works
  /// offline) and, in parallel, on the server (`/device/guard-sessions`).
  /// The server is authoritative when it answers within [serverCheckWait];
  /// otherwise the local result decides and a late server rejection locks
  /// the tablet again.
  Future<UnlockResult> unlockWithPin(RosterGuard g, String pin) async {
    final wait = pinAttempts.waitSeconds(g.id);
    if (wait > 0) return UnlockResult(UnlockStatus.locked, waitSeconds: wait);
    if (rosterStale) {
      await syncReference();
      if (rosterStale) return const UnlockResult(UnlockStatus.rosterStale);
    }
    final server = guardSessions.verify(g.id, pin);
    final local = g.hasPin ? PinVerifier.verify(pin, g.pinHash!) : Future.value(false);
    final r = await server.timeout(serverCheckWait, onTimeout: () => const GuardCheckResult(GuardCheck.unavailable, attemptsLeft: -1));
    final localOk = await local;
    final late = r.kind == GuardCheck.unavailable && r.attemptsLeft == -1;

    switch (r.kind) {
      case GuardCheck.verified:
        pinAttempts.success(g.id);
        if (!localOk) unawaited(syncReference()); // the cached hash is stale
        await unlock(g, verified: true);
        return const UnlockResult(UnlockStatus.unlocked);
      case GuardCheck.wrongPin:
        if (localOk) unawaited(syncReference()); // PIN changed on the server
        final left = pinAttempts.fail(g.id);
        final serverLeft = r.attemptsLeft;
        final n = serverLeft == null ? left : (serverLeft < left ? serverLeft : left);
        if (n <= 0) return UnlockResult(UnlockStatus.locked, waitSeconds: pinAttempts.waitSeconds(g.id));
        return UnlockResult(UnlockStatus.wrongPin, attemptsLeft: n);
      case GuardCheck.locked:
        pinAttempts.lockFor(g.id, r.retryAfter ?? const Duration(seconds: 30));
        return UnlockResult(UnlockStatus.locked, waitSeconds: pinAttempts.waitSeconds(g.id));
      case GuardCheck.notAllowed:
        unawaited(syncReference());
        return const UnlockResult(UnlockStatus.notAllowed);
      case GuardCheck.pinNotSet:
        unawaited(syncReference());
        return const UnlockResult(UnlockStatus.pinNotSet);
      case GuardCheck.unavailable:
        if (!localOk) {
          final left = pinAttempts.fail(g.id);
          if (left <= 0) return UnlockResult(UnlockStatus.locked, waitSeconds: pinAttempts.waitSeconds(g.id));
          return UnlockResult(UnlockStatus.wrongPin, attemptsLeft: left);
        }
        pinAttempts.success(g.id);
        // Offline unlock: no token unless the server still confirms it.
        if (!late) await guardSessions.drop(g.id);
        await unlock(g, verified: false);
        if (late) unawaited(server.then((res) => _lateServerAnswer(g, res)));
        return const UnlockResult(UnlockStatus.unlocked);
    }
  }

  /// The server answered after the tablet was unlocked locally.
  Future<void> _lateServerAnswer(RosterGuard g, GuardCheckResult r) async {
    if (guard?.id != g.id || phase != Phase.unlocked) return;
    switch (r.kind) {
      case GuardCheck.verified:
        guardVerified = true;
        notifyListeners();
      case GuardCheck.wrongPin || GuardCheck.locked || GuardCheck.notAllowed || GuardCheck.pinNotSet:
        if (r.kind == GuardCheck.wrongPin) pinAttempts.fail(g.id);
        if (r.kind == GuardCheck.locked) pinAttempts.lockFor(g.id, r.retryAfter ?? const Duration(seconds: 30));
        lockNotice = {'code': r.kind.name, 'name': g.name};
        lock();
        unawaited(syncReference());
      case GuardCheck.unavailable:
        await guardSessions.drop(g.id);
    }
  }

  /// Opens the guard's session (after the PIN was accepted).
  Future<void> unlock(RosterGuard g, {bool verified = false}) async {
    guard = g;
    guardStatus = null;
    guardVerified = verified;
    lockNotice = null;
    _lastActivity = DateTime.now();
    final cached = await db.getMap(CacheKeys.guardStatus(g.id));
    if (cached != null) guardStatus = GuardStatus.fromJson(cached);
    _go(Phase.unlocked);
    unawaited(refreshGuardStatus());
  }

  /// Current guard session token of the acting guard (null when unverified).
  String? get guardToken => guard == null || !guardVerified ? null : guardSessions.cached(guard!.id);

  /// The server rejected [guardId]'s token (outbox or a direct call).
  void onGuardTokenInvalid(String guardId) {
    unawaited(guardSessions.drop(guardId));
    if (guard?.id == guardId && guardVerified) {
      guardVerified = false;
      notifyListeners();
    }
  }

  void lock() {
    guard = null;
    guardStatus = null;
    guardVerified = false;
    patrolRunning = false;
    if (phase == Phase.unlocked) _go(Phase.locked);
  }

  void clearLockNotice() {
    if (lockNotice == null) return;
    lockNotice = null;
    notifyListeners();
  }

  /// Records user activity (any touch) for the idle auto-lock.
  void touch() => _lastActivity = DateTime.now();

  Duration get idleLimit {
    var limit = Duration(minutes: siteConfig.idleLockMinutes.clamp(1, 120));
    if (patrolRunning && limit < const Duration(minutes: 30)) limit = const Duration(minutes: 30);
    // Typing on the soft keyboard produces no pointer events: give a guard who
    // is writing a report time to finish instead of locking and losing it.
    if (_editingText && limit < const Duration(minutes: 15)) limit = const Duration(minutes: 15);
    return limit;
  }

  static bool get _editingText {
    final ctx = FocusManager.instance.primaryFocus?.context;
    return ctx != null && ctx.findAncestorWidgetOfExactType<EditableText>() != null;
  }

  Future<void> refreshGuardStatus() async {
    final g = guard;
    if (g == null) return;
    Future<dynamic> call(String? token) async {
      final sent = DateTime.now();
      final res = await api.get('/device/status', guardId: g.id, guardToken: token);
      final st = res is Map ? DateTime.tryParse('${res['server_time']}') : null;
      if (st != null) AppClock.observeServerTime(st, sent, DateTime.now());
      return res;
    }

    try {
      dynamic res;
      try {
        res = await call(guardToken);
      } on ApiException catch (e) {
        if (e.code != 'guard_token_invalid') rethrow;
        onGuardTokenInvalid(g.id);
        res = await call(null);
      }
      if (guard?.id != g.id) return;
      final m = (res as Map).cast<String, dynamic>();
      guardStatus = GuardStatus.fromJson(m);
      // The token expired meanwhile: show the guard as unverified.
      if (guardVerified && guardStatus!.guardVerified == false) onGuardTokenInvalid(g.id);
      await db.put(CacheKeys.guardStatus(g.id), m);
      notifyListeners();
    } on ApiException catch (e) {
      if (e.code == 'auth_expired') return expire();
      if (e.code == 'guard_not_allowed' || e.code == 'guard_unknown') {
        await syncReference();
        lock();
      }
      if (e.status == 403 && (e.code ?? '').startsWith('device_')) unawaited(refreshDevice());
    }
  }

  Future<void> refreshSos() async {
    try {
      final res = await api.get('/device/sos');
      sosAlerts = ((res as List?) ?? const []).map((e) => SosAlert.fromJson((e as Map).cast<String, dynamic>())).toList();
      notifyListeners();
    } on ApiException catch (e) {
      if (e.code == 'auth_expired') return expire();
    }
  }

  // ── timers ─────────────────────────────────────────────────────────────────

  /// How often SOS alerts of the site are polled now (null: not polled).
  Duration? get sosPollInterval {
    if (phase != Phase.unlocked && phase != Phase.locked) return null;
    if (pushHealthy?.call() ?? false) return sosPollPush;
    return phase == Phase.unlocked ? sosPollUnlocked : sosPollLocked;
  }

  static const _tickSeconds = 5;

  void _startTicker() {
    _ticker?.cancel();
    _ticker = Timer.periodic(const Duration(seconds: _tickSeconds), (_) => _onTick());
  }

  void _onTick() {
    _tick++;
    if (phase == Phase.unlocked && DateTime.now().difference(_lastActivity) > idleLimit) lock();
    if (phase == Phase.pending && _tick % 2 == 0) unawaited(refreshDevice()); // every 10 s
    if (phase == Phase.revoked && _tick % 12 == 0) unawaited(refreshDevice()); // re-activated by an admin?
    // Supervisors on duty see SOS of other tablets ([sosPollInterval]).
    final sosEvery = sosPollInterval;
    if (sosEvery != null && _tick % (sosEvery.inSeconds ~/ _tickSeconds) == 0) unawaited(refreshSos());
    if ((phase == Phase.locked || phase == Phase.unlocked) && _tick % 60 == 0) unawaited(syncReference()); // every 5 min
    if (phase == Phase.unlocked && _tick % 12 == 0) unawaited(refreshGuardStatus()); // every minute
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    switch (state) {
      case AppLifecycleState.paused || AppLifecycleState.hidden:
        // The camera (photo evidence) is opened on purpose: not "away".
        _backgroundAt ??= ExternalActivity.active ? null : DateTime.now();
      case AppLifecycleState.resumed:
        final away = _backgroundAt;
        _backgroundAt = null;
        if (phase != Phase.unlocked) return;
        if (away != null && DateTime.now().difference(away) > backgroundLock) return lock();
        if (DateTime.now().difference(_lastActivity) > idleLimit) lock();
      default:
        break;
    }
  }

  // ── sign-out / expiry ──────────────────────────────────────────────────────

  /// The offline token is no longer valid (device account disabled or the
  /// session revoked): sign in again.
  Future<void> expire() async {
    // Already signed out (e.g. the outbox reports it again on every tick):
    // don't pull the operator away from the setup/login screens.
    if (phase == Phase.setup || phase == Phase.login || phase == Phase.booting) return;
    await auth.signOut();
    guard = null;
    guardVerified = false;
    _go(Phase.login);
  }

  /// Removes the device account and all local data from the tablet. Refused
  /// with [OutboxNotEmpty] while items are queued (MOB-2): they are the only
  /// copy of field records.
  Future<void> signOutDevice() async {
    final n = await db.outboxCount();
    if (n > 0) throw OutboxNotEmpty(n);
    try {
      await beforeSignOut?.call();
    } catch (_) {}
    await auth.signOut();
    await guardSessions.clearAll();
    await db.wipe();
    device = null;
    site = null;
    roster = const [];
    routes = const [];
    guard = null;
    guardVerified = false;
    referenceSyncedAt = null;
    _go(Phase.login);
  }

  void _go(Phase p) {
    phase = p;
    notifyListeners();
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _ticker?.cancel();
    super.dispose();
  }
}
