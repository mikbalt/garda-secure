import 'dart:io' show HttpDate;

import 'package:timezone/data/latest_all.dart' as tzdata;
import 'package:timezone/timezone.dart' as tz;

/// The tablet's notion of "now" and of the site's wall clock.
///
/// **Server time (MOB-4).** Tablet clocks drift or are set wrong; the server
/// rejects event times outside [now − 7 days, now + 2 min] and flags late or
/// "ahead" times. Every successful API response updates [offset]
/// (`server − device`): precisely from `server_time` of `/device/status`,
/// coarsely (1 s) from the HTTP `Date` header otherwise. All event timestamps
/// use [now]; the outbox orders by a local sequence, not by time.
///
/// **Site time zone (MOB-9, D-013).** Times are shown in the site's IANA zone
/// ([site]) whatever the tablet's own zone setting; stored times stay UTC.
class AppClock {
  AppClock._();

  /// Server time minus device time.
  static Duration offset = Duration.zero;

  /// When [offset] was last confirmed by the server (device time).
  static DateTime? syncedAt;

  /// Called when [offset] changes noticeably (persisted by the session).
  static void Function(Duration offset)? onChanged;

  static bool _precise = false;

  /// Beyond this the Home screen warns about the tablet clock.
  static const warnAbove = Duration(minutes: 2);

  /// Server-corrected current time (UTC).
  static DateTime now() => DateTime.now().toUtc().add(offset);

  static bool get skewed => offset.abs() > warnAbove;

  /// Restores a persisted offset at start-up (until the server is reached).
  static void restore(Duration d) {
    offset = d;
    _precise = false;
  }

  /// Precise sample: `server_time` from a response that was requested at
  /// [sent] and received at [received] (device clock).
  static void observeServerTime(DateTime server, DateTime sent, DateTime received) {
    final mid = sent.add(received.difference(sent) ~/ 2);
    _set(server.toUtc().difference(mid.toUtc()), precise: true);
  }

  /// Coarse sample from an HTTP `Date` header (whole seconds).
  static void observeDateHeader(String? header, DateTime sent, DateTime received) {
    if (header == null || header.isEmpty) return;
    final DateTime server;
    try {
      server = HttpDate.parse(header).add(const Duration(milliseconds: 500));
    } catch (_) {
      return;
    }
    final mid = sent.add(received.difference(sent) ~/ 2);
    final candidate = server.toUtc().difference(mid.toUtc());
    // A precise sample is better unless the device clock moved since.
    if (_precise && (candidate - offset).abs() < const Duration(seconds: 3)) {
      syncedAt = DateTime.now();
      return;
    }
    _set(candidate, precise: false);
  }

  static void _set(Duration d, {required bool precise}) {
    final changed = (d - offset).abs() > const Duration(milliseconds: 500);
    syncedAt = DateTime.now();
    _precise = precise || (_precise && !changed);
    if (!changed) return;
    offset = d;
    onChanged?.call(d);
  }

  // ── site time zone ─────────────────────────────────────────────────────────

  static bool _tzLoaded = false;
  static tz.Location? _zone;
  static String? zoneName;

  /// Sets the site's IANA zone (e.g. `Asia/Jakarta`); unknown names fall
  /// back to the device zone.
  static void setSiteZone(String? name) {
    if (name == zoneName) return;
    zoneName = name;
    if (name == null || name.isEmpty) {
      _zone = null;
      return;
    }
    if (!_tzLoaded) {
      tzdata.initializeTimeZones();
      _tzLoaded = true;
    }
    try {
      _zone = tz.getLocation(name);
    } catch (_) {
      _zone = null;
    }
  }

  /// [t] as the site's wall-clock time (for display only).
  static DateTime site(DateTime t) => _zone == null ? t.toLocal() : tz.TZDateTime.from(t, _zone!);

  /// The site's current wall-clock time.
  static DateTime siteNow() => site(now());

  /// For tests.
  static void reset() {
    offset = Duration.zero;
    syncedAt = null;
    onChanged = null;
    _precise = false;
    _zone = null;
    zoneName = null;
  }
}
