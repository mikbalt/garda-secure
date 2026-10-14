import 'dart:async';
import 'dart:io';

import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';

import '../api/api_client.dart';
import '../api/tls.dart';
import '../database/local_db.dart';
import 'clock.dart';

/// App version from the platform (Android `versionName`/`versionCode`, set
/// from pubspec `version:` and the CI build number).
class AppInfo {
  AppInfo._();

  static String version = 'unknown';
  static String build = '';

  static const _channel = MethodChannel('securepatrol/app');

  static Future<void> load() async {
    try {
      final m = await _channel.invokeMapMethod<String, Object?>('version');
      version = (m?['name'] as String?) ?? version;
      build = (m?['code'] as String?) ?? build;
    } catch (_) {
      // iOS / tests: no channel.
    }
  }
}

/// Local crash/error log (MOB-7): a small ring buffer in the encrypted kv
/// cache, filled by `FlutterError.onError`, `PlatformDispatcher.onError` and
/// the root zone, and shipped in the health beacon. No external service.
class ErrorLog {
  ErrorLog(this._db);

  final LocalDb _db;
  static const max = 20;
  final List<Map<String, dynamic>> entries = [];

  Future<void> load() async {
    entries
      ..clear()
      ..addAll(await _db.getList(CacheKeys.errorLog));
  }

  void record(Object error, StackTrace? stack, {String source = 'zone'}) {
    String cut(String s, int n) => s.length <= n ? s : '${s.substring(0, n)}…';
    final trace = stack?.toString().split('\n').where((l) => l.trim().isNotEmpty).take(6).join('\n');
    entries.add({
      'at': DateTime.now().toUtc().toIso8601String(),
      'src': source,
      'error': cut(error.toString(), 300),
      if (trace != null && trace.isNotEmpty) 'stack': cut(trace, 800),
    });
    while (entries.length > max) {
      entries.removeAt(0);
    }
    unawaited(_db.put(CacheKeys.errorLog, entries).catchError((Object _) {}));
  }
}

/// What the beacon reports; filled by the app wiring (main.dart).
class HealthSnapshot {
  const HealthSnapshot({
    required this.outboxPending,
    required this.outboxFailed,
    this.outboxPaused,
    this.sosPending = false,
    this.lastSync,
    this.rosterSyncedAt,
    this.pinFailures = const {},
    this.dbKeyReset = false,
    this.push,
  });

  final int outboxPending;
  final int outboxFailed;
  final String? outboxPaused;
  final bool sosPending;
  final DateTime? lastSync;
  final DateTime? rosterSyncedAt;
  final Map<String, int> pinFailures;
  final bool dbKeyReset;

  /// Push status: `active`, `unconfigured`, `denied` or `error`.
  final String? push;
}

/// Compact device health beacon (MOB-7): app version, outbox depth, failed
/// items, last sync, clock offset and the last errors, sent every
/// [interval] while the device is active and online.
///
/// Endpoint: `POST /device/telemetry` (needs the backend; until it exists a
/// 404/405 slows the beacon down to every 6 h).
class HealthBeacon {
  HealthBeacon({required ApiClient api, required this.errors, required this.snapshot, required this.active, this.interval = const Duration(minutes: 15)})
      : _api = api;

  final ApiClient _api;
  final ErrorLog errors;
  final HealthSnapshot Function() snapshot;

  /// Whether the device may call field endpoints right now.
  final bool Function() active;
  final Duration interval;

  Timer? _timer;
  DateTime? _next;
  int _sentErrors = 0;

  static const path = '/device/telemetry';

  void start() {
    _timer?.cancel();
    _next = DateTime.now().add(const Duration(seconds: 30));
    _timer = Timer.periodic(const Duration(seconds: 30), (_) {
      if (_next != null && DateTime.now().isAfter(_next!)) unawaited(send());
    });
  }

  @visibleForTesting
  Map<String, dynamic> payload() {
    final s = snapshot();
    return {
      'app_version': AppInfo.version,
      'app_build': AppInfo.build,
      'platform': Platform.operatingSystem,
      'os_version': Platform.operatingSystemVersion,
      'outbox_pending': s.outboxPending,
      'outbox_failed': s.outboxFailed,
      if (s.outboxPaused != null) 'outbox_paused': s.outboxPaused,
      'sos_pending': s.sosPending,
      'last_sync': s.lastSync?.toUtc().toIso8601String(),
      'roster_synced_at': s.rosterSyncedAt?.toUtc().toIso8601String(),
      'clock_offset_ms': AppClock.offset.inMilliseconds,
      'private_ca': TrustStore.hasPrivateCa,
      if (s.pinFailures.isNotEmpty) 'pin_failures': s.pinFailures,
      if (s.dbKeyReset) 'db_key_reset': true,
      if (s.push != null) 'push': s.push,
      'errors': errors.entries.length > 10 ? errors.entries.sublist(errors.entries.length - 10) : errors.entries,
      'sent_at': AppClock.now().toIso8601String(),
    };
  }

  Future<void> send() async {
    if (!active()) {
      _next = DateTime.now().add(const Duration(minutes: 1));
      return;
    }
    _next = DateTime.now().add(interval);
    try {
      await _api.post(path, body: payload());
      _sentErrors = errors.entries.length;
    } on ApiException catch (e) {
      if (e.status == 404 || e.status == 405) {
        _next = DateTime.now().add(const Duration(hours: 6)); // endpoint not deployed yet
      } else if (e.network) {
        _next = DateTime.now().add(const Duration(minutes: 2));
      }
    }
  }

  /// Number of log entries included in the last delivered beacon.
  int get sentErrors => _sentErrors;

  void stop() => _timer?.cancel();
}
