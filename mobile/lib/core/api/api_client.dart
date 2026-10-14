import 'dart:io';

import 'package:dio/dio.dart';

import '../../config/env.dart';
import '../auth/keycloak_service.dart';
import '../services/clock.dart';
import 'tls.dart';

/// An API failure the UI can explain.
class ApiException implements Exception {
  const ApiException({this.status, this.code, required this.message, this.network = false, this.retryAfter, this.details = const {}});

  /// HTTP status, or null for network failures.
  final int? status;

  /// Backend machine code, e.g. `device_pending`, `no_shift`.
  final String? code;
  final String message;

  /// True when the server could not be reached (offline, timeout, DNS…).
  final bool network;

  /// `Retry-After` of a 429 (or `retry_after` in the body of a 423).
  final Duration? retryAfter;

  /// The whole error body (e.g. `attempts_left`, `field`).
  final Map<String, dynamic> details;

  /// Worth retrying later (offline, server error, throttling, token refresh).
  bool get retryable => network || status == null || status! >= 500 || status == 401 || status == 408 || status == 429;

  @override
  String toString() => 'ApiException(${status ?? 'network'} ${code ?? ''}: $message)';

  static Duration? _seconds(Object? v) {
    final n = v is num ? v.toInt() : (v is String ? int.tryParse(v.trim()) : null);
    return n == null || n < 0 ? null : Duration(seconds: n);
  }

  static ApiException from(Object e) {
    if (e is ApiException) return e;
    if (e is AuthExpired) return const ApiException(status: 401, code: 'auth_expired', message: 'device session expired');
    if (e is InsecureUrl) return const ApiException(code: 'insecure_url', message: 'https is required');
    if (e is DioException) {
      final res = e.response;
      if (res == null) {
        return ApiException(network: true, message: e.message ?? e.type.name);
      }
      final data = res.data;
      final body = data is Map ? data.cast<String, dynamic>() : const <String, dynamic>{};
      return ApiException(
        status: res.statusCode,
        code: body['code'] as String?,
        message: (body['error'] as String?) ?? 'HTTP ${res.statusCode}',
        retryAfter: _seconds(res.headers.value('retry-after')) ?? _seconds(body['retry_after']),
        details: body,
      );
    }
    if (e is SocketException || e is HttpException || e is HandshakeException) return ApiException(network: true, message: e.toString());
    return ApiException(message: e.toString());
  }
}

/// REST client for `/api/v1` with the device token, the acting guard and
/// (when the guard was verified online) the guard session token.
class ApiClient {
  ApiClient(this._auth, this._config, {Dio? dio})
      : _dio = dio ??
            TrustStore.dio(BaseOptions(
              connectTimeout: const Duration(seconds: 12),
              receiveTimeout: const Duration(seconds: 25),
              sendTimeout: const Duration(seconds: 60),
            ));

  final KeycloakService _auth;
  final ServerConfig Function() _config;
  final Dio _dio;

  ServerConfig get config => _config();

  Future<Map<String, String>> _headers(String? guardId, String? guardToken) async {
    final token = await _auth.accessToken(config);
    return {
      'Authorization': 'Bearer $token',
      if (guardId != null) 'X-Guard-ID': guardId,
      if (guardId != null && guardToken != null) 'X-Guard-Token': guardToken,
    };
  }

  /// Sends a request; retries once with a fresh token on 401. Every response
  /// updates the server clock offset (MOB-4).
  Future<dynamic> send(String method, String path, {Object? body, Map<String, dynamic>? query, String? guardId, String? guardToken, Duration? timeout}) async {
    Future<Response<dynamic>> go() async {
      final url = '${config.apiBase}$path';
      TrustStore.checkUrl(url);
      final sent = DateTime.now();
      try {
        final res = await _dio.request<dynamic>(
          url,
          // A multipart body can be sent only once: build it per attempt.
          data: body is FormData Function() ? body() : body,
          queryParameters: query,
          options: Options(method: method, headers: await _headers(guardId, guardToken), receiveTimeout: timeout, sendTimeout: timeout),
        );
        AppClock.observeDateHeader(res.headers.value('date'), sent, DateTime.now());
        return res;
      } on DioException catch (e) {
        final r = e.response;
        if (r != null) AppClock.observeDateHeader(r.headers.value('date'), sent, DateTime.now());
        rethrow;
      }
    }

    try {
      try {
        return (await go()).data;
      } on DioException catch (e) {
        final data = e.response?.data;
        // A 401 with `invalid_pin` is an answer (guard sessions), not an
        // expired device token: never resend a PIN check.
        if (e.response?.statusCode == 401 && !(data is Map && data['code'] == 'invalid_pin')) {
          await _auth.refresh(config);
          return (await go()).data;
        }
        rethrow;
      }
    } catch (e) {
      throw ApiException.from(e);
    }
  }

  Future<dynamic> get(String path, {Map<String, dynamic>? query, String? guardId, String? guardToken}) =>
      send('GET', path, query: query, guardId: guardId, guardToken: guardToken);

  Future<dynamic> post(String path, {Object? body, String? guardId, String? guardToken, Duration? timeout}) =>
      send('POST', path, body: body ?? const {}, guardId: guardId, guardToken: guardToken, timeout: timeout);

  Future<dynamic> put(String path, {Object? body, Duration? timeout}) => send('PUT', path, body: body ?? const {}, timeout: timeout);

  Future<dynamic> delete(String path, {Duration? timeout}) => send('DELETE', path, timeout: timeout);

  /// Uploads a photo with a client ID so the stored key is deterministic
  /// (`sites/<site>/<kind>/<id>.jpg`, D-012).
  Future<String> uploadPhoto({required String filePath, required String id, required String kind, String? guardId, String? guardToken}) async {
    FormData form() => FormData.fromMap({
          'id': id,
          'kind': kind,
          'file': MultipartFile.fromFileSync(filePath, filename: '$id.jpg'),
        });
    final res = await send('POST', '/media', body: form, guardId: guardId, guardToken: guardToken);
    return (res as Map)['object_key'] as String;
  }
}
