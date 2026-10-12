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

/// Locks the PIN pad after repeated failures (per guard, in memory).
class PinAttempts {
  PinAttempts({this.maxAttempts = 5, this.lockout = const Duration(seconds: 30)});

  final int maxAttempts;
  final Duration lockout;
  final Map<String, int> _fails = {};
  final Map<String, DateTime> _until = {};

  /// Seconds the guard must still wait, or 0.
  int waitSeconds(String guardId, [DateTime? now]) {
    final until = _until[guardId];
    if (until == null) return 0;
    final left = until.difference(now ?? DateTime.now()).inSeconds;
    if (left <= 0) {
      _until.remove(guardId);
      _fails.remove(guardId);
      return 0;
    }
    return left + 1;
  }

  /// Records a failure; returns attempts left before the lockout.
  int fail(String guardId, [DateTime? now]) {
    final n = (_fails[guardId] ?? 0) + 1;
    _fails[guardId] = n;
    if (n >= maxAttempts) {
      _until[guardId] = (now ?? DateTime.now()).add(lockout);
      return 0;
    }
    return maxAttempts - n;
  }

  void success(String guardId) {
    _fails.remove(guardId);
    _until.remove(guardId);
  }
}
