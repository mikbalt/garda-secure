import 'dart:async';
import 'package:dio/dio.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import '../../config/env.dart';

class KeycloakService {
  static const _accessTokenKey = 'access_token';
  static const _refreshTokenKey = 'refresh_token';
  static const _expiresAtKey = 'token_expires_at';

  final FlutterSecureStorage _storage = const FlutterSecureStorage();
  final Dio _dio = Dio();
  Timer? _refreshTimer;

  String get _tokenEndpoint =>
      '${Env.keycloakUrl}/realms/${Env.keycloakRealm}/protocol/openid-connect/token';

  /// Authenticate with username and password, store tokens, schedule refresh.
  Future<void> login(String username, String password) async {
    final response = await _dio.post(
      _tokenEndpoint,
      data: {
        'grant_type': 'password',
        'client_id': Env.keycloakClientId,
        'username': username,
        'password': password,
      },
      options: Options(
        contentType: 'application/x-www-form-urlencoded',
      ),
    );

    await _storeTokens(response.data as Map<String, dynamic>);
  }

  /// Refresh the access token using the stored refresh token.
  Future<void> refreshToken() async {
    final storedRefresh = await _storage.read(key: _refreshTokenKey);
    if (storedRefresh == null) {
      throw Exception('No refresh token available');
    }

    final response = await _dio.post(
      _tokenEndpoint,
      data: {
        'grant_type': 'refresh_token',
        'client_id': Env.keycloakClientId,
        'refresh_token': storedRefresh,
      },
      options: Options(
        contentType: 'application/x-www-form-urlencoded',
      ),
    );

    await _storeTokens(response.data as Map<String, dynamic>);
  }

  /// Clear all stored tokens and cancel refresh timer.
  Future<void> logout() async {
    _refreshTimer?.cancel();
    _refreshTimer = null;
    await _storage.delete(key: _accessTokenKey);
    await _storage.delete(key: _refreshTokenKey);
    await _storage.delete(key: _expiresAtKey);
  }

  /// Retrieve the stored access token, refreshing if needed.
  Future<String?> getAccessToken() async {
    final expiresAtStr = await _storage.read(key: _expiresAtKey);
    if (expiresAtStr != null) {
      final expiresAt = DateTime.parse(expiresAtStr);
      if (DateTime.now().isAfter(expiresAt.subtract(const Duration(seconds: 30)))) {
        try {
          await refreshToken();
        } catch (_) {
          return null;
        }
      }
    }
    return _storage.read(key: _accessTokenKey);
  }

  /// Returns true if a valid (non-expired) access token exists.
  Future<bool> isAuthenticated() async {
    final token = await _storage.read(key: _accessTokenKey);
    if (token == null) return false;

    final expiresAtStr = await _storage.read(key: _expiresAtKey);
    if (expiresAtStr == null) return false;

    final expiresAt = DateTime.parse(expiresAtStr);
    return DateTime.now().isBefore(expiresAt);
  }

  Future<void> _storeTokens(Map<String, dynamic> data) async {
    final accessToken = data['access_token'] as String;
    final refreshToken = data['refresh_token'] as String?;
    final expiresIn = data['expires_in'] as int;

    final expiresAt = DateTime.now().add(Duration(seconds: expiresIn));

    await _storage.write(key: _accessTokenKey, value: accessToken);
    if (refreshToken != null) {
      await _storage.write(key: _refreshTokenKey, value: refreshToken);
    }
    await _storage.write(
        key: _expiresAtKey, value: expiresAt.toIso8601String());

    // Schedule proactive refresh at (expiresIn - 60) seconds
    _refreshTimer?.cancel();
    final refreshDelay = expiresIn - 60;
    if (refreshDelay > 0) {
      _refreshTimer = Timer(Duration(seconds: refreshDelay), () async {
        try {
          await refreshToken();
        } catch (_) {
          // Refresh failed; user will be prompted to re-login
        }
      });
    }
  }
}
