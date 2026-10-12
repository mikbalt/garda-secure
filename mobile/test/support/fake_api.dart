import 'package:securepatrol/config/env.dart';
import 'package:securepatrol/core/api/api_client.dart';
import 'package:securepatrol/core/auth/keycloak_service.dart';
import 'package:securepatrol/core/storage/secure_store.dart';

/// One recorded request.
class Call {
  Call(this.method, this.path, this.body, this.guardId, this.guardToken);

  final String method;
  final String path;
  final Object? body;
  final String? guardId;
  final String? guardToken;

  Map<String, dynamic> get json => body is Map ? Map<String, dynamic>.of(body as Map<String, dynamic>) : const {};
}

/// An [ApiClient] without network. Each request is answered by [handler]
/// (return a value, or throw an [ApiException]); without a handler the next
/// entry of [script] (null = 200 `{}`), then 200 `{}`.
class FakeApi extends ApiClient {
  FakeApi({List<Object?>? script, this.handler})
      : script = script ?? [],
        super(KeycloakService(MemorySecureStore()), () => const ServerConfig(serverUrl: 'https://patrol.test', realm: 'demo'));

  /// Entries: null (200 {}), an [ApiException] (thrown) or a response value.
  final List<Object?> script;
  Future<Object?> Function(Call call)? handler;
  final calls = <Call>[];

  /// Answer every request with "no network".
  static FakeApi offline() => FakeApi(handler: (_) async => throw const ApiException(network: true, message: 'offline'));

  @override
  Future<dynamic> send(String method, String path, {Object? body, Map<String, dynamic>? query, String? guardId, String? guardToken, Duration? timeout}) async {
    final call = Call(method, path, body is Map ? Map<String, dynamic>.of(body.cast<String, dynamic>()) : body, guardId, guardToken);
    calls.add(call);
    if (handler != null) return handler!(call);
    final next = script.isEmpty ? null : script.removeAt(0);
    if (next is ApiException) throw next;
    return next ?? const {};
  }

  @override
  Future<String> uploadPhoto({required String filePath, required String id, required String kind, String? guardId, String? guardToken}) async {
    final res = await send('POST', '/media', body: {'id': id, 'kind': kind, 'file': filePath}, guardId: guardId, guardToken: guardToken);
    return res is Map && res['object_key'] is String ? res['object_key'] as String : 'sites/s/$kind/$id.jpg';
  }

  List<String> get paths => calls.map((c) => c.path).toList();
}
