import 'dart:io';

import 'package:dio/dio.dart';

import '../../config/env.dart';
import '../auth/keycloak_service.dart';

/// An API failure the UI can explain.
class ApiException implements Exception {
  const ApiException({this.status, this.code, required this.message, this.network = false});

  /// HTTP status, or null for network failures.
  final int? status;

  /// Backend machine code, e.g. `device_pending`, `no_shift`.
  final String? code;
  final String message;

  /// True when the server could not be reached (offline, timeout, DNS…).
  final bool network;

  /// Worth retrying later (offline, server error, throttling, token refresh).
  bool get retryable => network || status == null || status! >= 500 || status == 401 || status == 408 || status == 429;

  @override
  String toString() => 'ApiException(${status ?? 'network'} ${code ?? ''}: $message)';

  static ApiException from(Object e) {
    if (e is ApiException) return e;
    if (e is AuthExpired) return const ApiException(status: 401, code: 'auth_expired', message: 'device session expired');
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
      );
    }
    if (e is SocketException || e is HttpException) return ApiException(network: true, message: e.toString());
    return ApiException(message: e.toString());
  }
}

/// REST client for `/api/v1` with the device token and the acting guard.
class ApiClient {
  ApiClient(this._auth, this._config, {Dio? dio})
      : _dio = dio ??
            Dio(BaseOptions(
              connectTimeout: const Duration(seconds: 12),
              receiveTimeout: const Duration(seconds: 25),
              sendTimeout: const Duration(seconds: 60),
            ));

  final KeycloakService _auth;
  final ServerConfig Function() _config;
  final Dio _dio;

  ServerConfig get config => _config();

  Future<Map<String, String>> _headers(String? guardId) async {
    final token = await _auth.accessToken(config);
    return {
      'Authorization': 'Bearer $token',
      if (guardId != null) 'X-Guard-ID': guardId,
    };
  }

  /// Sends a request; retries once with a fresh token on 401.
  Future<dynamic> send(String method, String path, {Object? body, Map<String, dynamic>? query, String? guardId}) async {
    Future<Response<dynamic>> go() async => _dio.request<dynamic>(
          '${config.apiBase}$path',
          data: body,
          queryParameters: query,
          options: Options(method: method, headers: await _headers(guardId)),
        );
    try {
      try {
        return (await go()).data;
      } on DioException catch (e) {
        if (e.response?.statusCode == 401) {
          await _auth.refresh(config);
          return (await go()).data;
        }
        rethrow;
      }
    } catch (e) {
      throw ApiException.from(e);
    }
  }

  Future<dynamic> get(String path, {Map<String, dynamic>? query, String? guardId}) => send('GET', path, query: query, guardId: guardId);

  Future<dynamic> post(String path, {Object? body, String? guardId}) => send('POST', path, body: body ?? const {}, guardId: guardId);

  /// Uploads a photo with a client ID so the stored key is deterministic
  /// (`sites/<site>/<kind>/<id>.jpg`, D-012).
  Future<String> uploadPhoto({required String filePath, required String id, required String kind, String? guardId}) async {
    final form = FormData.fromMap({
      'id': id,
      'kind': kind,
      'file': await MultipartFile.fromFile(filePath, filename: '$id.jpg'),
    });
    final res = await send('POST', '/media', body: form, guardId: guardId);
    return (res as Map)['object_key'] as String;
  }
}
