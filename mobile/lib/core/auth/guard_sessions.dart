import 'dart:convert';

import '../api/api_client.dart';
import '../services/clock.dart';
import '../storage/secure_store.dart';

/// A short-lived, server-signed proof that a guard entered the right PIN
/// online (D-035). Sent as `X-Guard-Token` with the guard's requests.
class GuardToken {
  const GuardToken(this.token, this.expiresAt);

  final String token;
  final DateTime expiresAt;

  /// Usable for at least another 30 s.
  bool get valid => expiresAt.isAfter(AppClock.now().add(const Duration(seconds: 30)));

  Map<String, dynamic> toJson() => {'token': token, 'expires_at': expiresAt.toUtc().toIso8601String()};

  static GuardToken? fromJson(Map<String, dynamic> j) {
    final t = j['token'], e = j['expires_at'];
    if (t is! String || t.isEmpty || e is! String) return null;
    final at = DateTime.tryParse(e);
    return at == null ? null : GuardToken(t, at.toUtc());
  }
}

enum GuardCheck {
  /// 200: PIN right, token issued.
  verified,

  /// 401 `invalid_pin`.
  wrongPin,

  /// 423 `pin_locked`.
  locked,

  /// 403 `guard_not_allowed` (or unknown guard).
  notAllowed,

  /// 409 `pin_not_set`.
  pinNotSet,

  /// Server not reachable / endpoint missing / server error: decide locally.
  unavailable,
}

class GuardCheckResult {
  const GuardCheckResult(this.kind, {this.token, this.attemptsLeft, this.retryAfter});

  final GuardCheck kind;
  final GuardToken? token;
  final int? attemptsLeft;
  final Duration? retryAfter;
}

/// Online PIN verification (`POST /device/guard-sessions`) and the per-guard
/// token cache (secure storage, with expiry).
class GuardSessions {
  GuardSessions(this._api, this._store);

  final ApiClient _api;
  final SecureStore _store;
  final Map<String, GuardToken> _cache = {};

  /// Asks the server to verify [pin] for [guardId].
  Future<GuardCheckResult> verify(String guardId, String pin, {Duration timeout = const Duration(seconds: 8)}) async {
    try {
      final res = await _api.post('/device/guard-sessions', body: {'guard_id': guardId, 'pin': pin}, timeout: timeout);
      final m = res is Map ? res.cast<String, dynamic>() : const <String, dynamic>{};
      final token = GuardToken.fromJson(m);
      if (token == null) return const GuardCheckResult(GuardCheck.unavailable);
      await save(guardId, token);
      return GuardCheckResult(GuardCheck.verified, token: token);
    } on ApiException catch (e) {
      switch (e.status) {
        case 401 when e.code == 'invalid_pin':
          final left = e.details['attempts_left'];
          return GuardCheckResult(GuardCheck.wrongPin, attemptsLeft: left is num ? left.toInt() : null);
        case 423:
          return GuardCheckResult(GuardCheck.locked, retryAfter: e.retryAfter ?? const Duration(seconds: 30));
        case 403 when e.code == 'guard_not_allowed' || e.code == 'guard_unknown':
          return const GuardCheckResult(GuardCheck.notAllowed);
        case 409 when e.code == 'pin_not_set':
          return const GuardCheckResult(GuardCheck.pinNotSet);
      }
      return const GuardCheckResult(GuardCheck.unavailable);
    } catch (_) {
      return const GuardCheckResult(GuardCheck.unavailable);
    }
  }

  Future<void> save(String guardId, GuardToken t) async {
    _cache[guardId] = t;
    await _store.write(SecureKeys.guardToken(guardId), jsonEncode(t.toJson()));
  }

  /// The guard's token if still valid (memory, then secure storage).
  Future<GuardToken?> tokenFor(String guardId) async {
    var t = _cache[guardId];
    if (t == null) {
      final raw = await _store.read(SecureKeys.guardToken(guardId));
      if (raw != null) {
        try {
          t = GuardToken.fromJson((jsonDecode(raw) as Map).cast<String, dynamic>());
        } catch (_) {}
      }
    }
    if (t == null || !t.valid) {
      if (t != null) await drop(guardId);
      return null;
    }
    _cache[guardId] = t;
    return t;
  }

  /// Synchronous lookup of a cached, still valid token.
  String? cached(String guardId) {
    final t = _cache[guardId];
    return t != null && t.valid ? t.token : null;
  }

  Future<void> drop(String guardId) async {
    _cache.remove(guardId);
    await _store.write(SecureKeys.guardToken(guardId), null);
  }

  Future<void> clearAll() async {
    _cache.clear();
    await _store.deletePrefix('sp.gt.');
  }
}
