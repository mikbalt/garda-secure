import 'package:bcrypt/bcrypt.dart';
import 'package:flutter/foundation.dart';

/// Offline PIN check against the bcrypt hash synced from the server (D-001).
/// bcrypt is deliberately slow (cost 12), so it runs on a background isolate
/// to keep the PIN pad responsive.
class PinVerifier {
  static Future<bool> verify(String pin, String hash) => compute(_check, [pin, hash]);

  static bool _check(List<String> args) {
    try {
      // Go's bcrypt writes $2a$; the Dart implementation accepts $2a/$2b/$2y.
      return BCrypt.checkpw(args[0], args[1]);
    } catch (_) {
      return false;
    }
  }

  /// Synchronous variant for tests.
  @visibleForTesting
  static bool verifySync(String pin, String hash) => _check([pin, hash]);
}

class _GuardLock {
  _GuardLock({this.fails = 0, this.level = 0, this.until, this.total = 0});

  /// Failures since the last success or lockout.
  int fails;

  /// Lockouts so far (escalation step).
  int level;
  DateTime? until;

  /// All failures since the last success (reported in the health beacon).
  int total;

  Map<String, dynamic> toJson() => {'f': fails, 'l': level, 'u': until?.toUtc().toIso8601String(), 't': total};

  factory _GuardLock.fromJson(Map<String, dynamic> j) => _GuardLock(
        fails: (j['f'] as num?)?.toInt() ?? 0,
        level: (j['l'] as num?)?.toInt() ?? 0,
        until: j['u'] is String ? DateTime.tryParse(j['u'] as String) : null,
        total: (j['t'] as num?)?.toInt() ?? 0,
      );
}

/// PIN lockout per guard (MOB-6), persisted so restarting the app does not
/// reset it. After [maxAttempts] wrong PINs the guard waits [steps].first;
/// every further wrong PIN before a success locks for the next, longer step
/// (30 s, 2 min, 10 min, 30 min, 1 h). The server keeps its own counter for
/// online checks (guard sessions) and is authoritative when reachable.
class PinAttempts {
  PinAttempts({
    this.maxAttempts = 5,
    this.steps = const [Duration(seconds: 30), Duration(minutes: 2), Duration(minutes: 10), Duration(minutes: 30), Duration(hours: 1)],
    this.onChanged,
  });

  final int maxAttempts;
  final List<Duration> steps;

  /// Called after every change with the state to persist.
  void Function(Map<String, dynamic> state)? onChanged;

  final Map<String, _GuardLock> _locks = {};

  void load(Map<String, dynamic>? state) {
    _locks.clear();
    state?.forEach((k, v) {
      if (v is Map) _locks[k] = _GuardLock.fromJson(v.cast<String, dynamic>());
    });
  }

  Map<String, dynamic> toJson() => {for (final e in _locks.entries) e.key: e.value.toJson()};

  void _changed() => onChanged?.call(toJson());

  /// Seconds the guard must still wait, or 0.
  int waitSeconds(String guardId, [DateTime? now]) {
    final l = _locks[guardId];
    final until = l?.until;
    if (until == null) return 0;
    final left = until.difference(now ?? DateTime.now()).inMilliseconds;
    if (left <= 0) return 0;
    return (left / 1000).ceil();
  }

  /// Records a failure; returns attempts left before the next lockout
  /// (0 = locked now).
  int fail(String guardId, [DateTime? now]) {
    final t = now ?? DateTime.now();
    final l = _locks.putIfAbsent(guardId, _GuardLock.new);
    l.total++;
    if (l.level == 0) {
      l.fails++;
      if (l.fails < maxAttempts) {
        _changed();
        return maxAttempts - l.fails;
      }
    }
    l.until = t.add(steps[l.level.clamp(0, steps.length - 1)]);
    l.level++;
    l.fails = 0;
    _changed();
    return 0;
  }

  /// The server locked the guard (423 `pin_locked`).
  void lockFor(String guardId, Duration d, [DateTime? now]) {
    final l = _locks.putIfAbsent(guardId, _GuardLock.new);
    final until = (now ?? DateTime.now()).add(d);
    if (l.until == null || until.isAfter(l.until!)) l.until = until;
    if (l.level == 0) l.level = 1;
    _changed();
  }

  void success(String guardId) {
    if (_locks.remove(guardId) != null) _changed();
  }

  /// Guards with failed attempts since their last success: `{guardId: n}`.
  Map<String, int> get failureCounts => {for (final e in _locks.entries) if (e.value.total > 0) e.key: e.value.total};
}
