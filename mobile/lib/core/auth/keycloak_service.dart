import 'dart:async';
import 'dart:convert';

import 'package:dio/dio.dart';

import '../../config/env.dart';
import '../storage/secure_store.dart';

/// Thrown when the device must sign in again (offline token revoked/expired).
class AuthExpired implements Exception {
  const AuthExpired([this.message = 'device session expired']);
  final String message;
  @override
  String toString() => message;
}

/// Thrown by [KeycloakService.login] with a reason the UI can translate.
class LoginFailure implements Exception {
  const LoginFailure(this.reason, [this.detail]);

  /// `bad_credentials`, `not_device`, `unreachable` or `server`.
  final String reason;
  final String? detail;
  @override
  String toString() => 'LoginFailure($reason${detail != null ? ': $detail' : ''})';
}

/// Device authentication against Keycloak (D-001): the tablet signs in once
/// with its device account using the password grant and keeps an **offline
/// token**, so it stays signed in across restarts until an admin revokes it.
class KeycloakService {
  KeycloakService(this._store, {Dio? dio})
      : _dio = dio ?? Dio(BaseOptions(connectTimeout: const Duration(seconds: 15), receiveTimeout: const Duration(seconds: 20)));

  final SecureStore _store;
  final Dio _dio;
  Future<String>? _refreshing;

  Future<void> login(ServerConfig cfg, String username, String password) async {
    final Response<dynamic> res;
    try {
      res = await _dio.post<dynamic>(
        cfg.tokenEndpoint,
        data: {
          'grant_type': 'password',
          'client_id': cfg.clientId,
          'username': username.trim(),
          'password': password,
          'scope': 'openid offline_access',
        },
        options: Options(contentType: Headers.formUrlEncodedContentType, validateStatus: (_) => true),
      );
    } on DioException catch (e) {
      throw LoginFailure('unreachable', e.message);
    }
    if (res.statusCode == 401 || res.statusCode == 400) {
      throw const LoginFailure('bad_credentials');
    }
    if (res.statusCode != 200 || res.data is! Map) {
      throw LoginFailure('server', 'status ${res.statusCode}');
    }
    final data = (res.data as Map).cast<String, dynamic>();
    final roles = rolesOf(data['access_token'] as String? ?? '');
    if (!roles.contains('device')) {
      throw const LoginFailure('not_device');
    }
    await _save(data);
  }

  /// Returns a valid access token, refreshing it with the offline token when
  /// it expires within 30 s. Concurrent callers share one refresh.
  Future<String> accessToken(ServerConfig cfg) async {
    final token = await _store.read(SecureKeys.accessToken);
    final exp = DateTime.tryParse(await _store.read(SecureKeys.accessExpiry) ?? '');
    if (token != null && exp != null && DateTime.now().isBefore(exp.subtract(const Duration(seconds: 30)))) {
      return token;
    }
    return _refreshing ??= _refresh(cfg).whenComplete(() => _refreshing = null);
  }

  /// Forces a refresh (e.g. after the API answered 401).
  Future<String> refresh(ServerConfig cfg) => _refreshing ??= _refresh(cfg).whenComplete(() => _refreshing = null);

  Future<String> _refresh(ServerConfig cfg) async {
    final refresh = await _store.read(SecureKeys.refreshToken);
    if (refresh == null) throw const AuthExpired();
    final Response<dynamic> res;
    try {
      res = await _dio.post<dynamic>(
        cfg.tokenEndpoint,
        data: {'grant_type': 'refresh_token', 'client_id': cfg.clientId, 'refresh_token': refresh},
        options: Options(contentType: Headers.formUrlEncodedContentType, validateStatus: (_) => true),
      );
    } on DioException {
      rethrow; // offline: the caller retries later
    }
    if (res.statusCode == 400 || res.statusCode == 401) {
      throw const AuthExpired();
    }
    if (res.statusCode != 200 || res.data is! Map) {
      throw DioException(requestOptions: res.requestOptions, response: res, type: DioExceptionType.badResponse);
    }
    final data = (res.data as Map).cast<String, dynamic>();
    await _save(data);
    return data['access_token'] as String;
  }

  Future<bool> get hasSession async => (await _store.read(SecureKeys.refreshToken)) != null;

  Future<void> signOut() async {
    await _store.write(SecureKeys.accessToken, null);
    await _store.write(SecureKeys.accessExpiry, null);
    await _store.write(SecureKeys.refreshToken, null);
  }

  Future<void> _save(Map<String, dynamic> data) async {
    final expiresIn = (data['expires_in'] as num?)?.toInt() ?? 300;
    await _store.write(SecureKeys.accessToken, data['access_token'] as String?);
    await _store.write(SecureKeys.accessExpiry, DateTime.now().add(Duration(seconds: expiresIn)).toIso8601String());
    final refresh = data['refresh_token'] as String?;
    if (refresh != null) await _store.write(SecureKeys.refreshToken, refresh);
  }

  /// Realm roles in an access token (flat `roles` claim or `realm_access.roles`).
  static Set<String> rolesOf(String jwt) {
    final parts = jwt.split('.');
    if (parts.length < 2) return {};
    try {
      final payload = jsonDecode(utf8.decode(base64Url.decode(base64Url.normalize(parts[1])))) as Map<String, dynamic>;
      final roles = <String>{};
      void add(Object? v) {
        if (v is List) roles.addAll(v.whereType<String>());
      }

      add(payload['roles']);
      final ra = payload['realm_access'];
      if (ra is Map) add(ra['roles']);
      return roles;
    } catch (_) {
      return {};
    }
  }
}
