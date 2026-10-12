import 'dart:async';

import 'package:connectivity_plus/connectivity_plus.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:securepatrol/config/env.dart';
import 'package:securepatrol/core/api/api_client.dart';
import 'package:securepatrol/core/auth/keycloak_service.dart';
import 'package:securepatrol/core/database/local_db.dart';
import 'package:securepatrol/core/database/sync_service.dart';
import 'package:securepatrol/core/storage/secure_store.dart';
import 'package:sqflite_common_ffi/sqflite_ffi.dart';

/// Answers each request with the next scripted result (null = 200 OK).
class ScriptedApi extends ApiClient {
  ScriptedApi(this.script) : super(KeycloakService(MemorySecureStore()), () => const ServerConfig(serverUrl: 'https://patrol.test', realm: 'demo'));

  final List<ApiException?> script;
  final sent = <Map<String, dynamic>>[];

  @override
  Future<dynamic> send(String method, String path, {Object? body, Map<String, dynamic>? query, String? guardId}) async {
    sent.add({'path': path, 'body': Map<String, dynamic>.of(body as Map<String, dynamic>)});
    final next = script.isEmpty ? null : script.removeAt(0);
    if (next != null) throw next;
    return const {};
  }
}

OutboxOp op(String id, String kind, {int priority = 1, Map<String, dynamic>? body}) => OutboxOp(
      id: id,
      kind: kind,
      method: 'POST',
      path: '/$kind',
      guardId: 'g-1',
      priority: priority,
      body: body ?? {'id': id},
    );

void main() {
  late LocalDb db;
  setUp(() async {
    sqfliteFfiInit();
    db = await LocalDb.open(factory: databaseFactoryFfi, path: inMemoryDatabasePath);
  });
  tearDown(() => db.db.close());

  Outbox outboxWith(ScriptedApi api, {Stream<List<ConnectivityResult>>? connectivity}) =>
      Outbox(db, api, connectivity: connectivity ?? const Stream<List<ConnectivityResult>>.empty(), tick: const Duration(hours: 1));

  test('an SOS rejected by the server is never parked', () async {
    final api = ScriptedApi([const ApiException(status: 403, code: 'guard_not_allowed', message: 'no')]);
    final o = outboxWith(api);
    await o.enqueue(op('s-1', 'sos', priority: 0));
    await o.process();
    expect(o.failed, 0);
    expect(o.sosPending, isTrue);
    expect(o.sosRejected, isTrue);
    o.dispose();
  });

  test('an SOS rejected for clock skew is resent at once without the client time', () async {
    final api = ScriptedApi([const ApiException(status: 400, code: 'timestamp_out_of_range', message: 'skew')]);
    final o = outboxWith(api);
    await o.enqueue(op('s-1', 'sos', priority: 0, body: {'id': 's-1', 'fired_at': '2020-01-01T00:00:00Z'}));
    await o.process();
    expect(api.sent, hasLength(2));
    expect(api.sent.last['body'], {'id': 's-1'});
    expect(o.sosPending, isFalse);
    expect(o.sosRejected, isFalse);
    o.dispose();
  });

  test('other rejected operations are parked and the queue moves on', () async {
    final api = ScriptedApi([const ApiException(status: 400, code: 'no_shift', message: 'no shift')]);
    final o = outboxWith(api);
    await o.enqueue(op('a', 'clock_in'), send: false);
    await o.enqueue(op('b', 'incident'), send: false);
    await o.process();
    expect(o.failed, 1);
    expect(o.pending, 0);
    expect(api.sent.map((r) => r['path']), ['/clock_in', '/incident']);
    o.dispose();
  });

  test('coming back online retries at once instead of waiting out the backoff', () async {
    final conn = StreamController<List<ConnectivityResult>>();
    final api = ScriptedApi([for (var i = 0; i < 6; i++) const ApiException(network: true, message: 'offline')]);
    final o = outboxWith(api, connectivity: conn.stream);
    await o.start();
    await o.enqueue(op('a', 'clock_in'), send: false);
    for (var i = 0; i < 6; i++) {
      await o.flush(); // six failures: the backoff is now 10 minutes
    }
    api.script.clear();
    conn.add([ConnectivityResult.wifi]);
    await Future<void>.delayed(const Duration(milliseconds: 50));
    await o.process();
    expect(o.pending, 0);
    o.dispose();
    await conn.close();
  });

  test('late GPS is attached to an SOS that was not sent yet', () async {
    final o = outboxWith(ScriptedApi([]));
    await o.enqueue(op('s-1', 'sos', priority: 0), send: false);
    await o.amendBody('s-1', {'gps_lat': -6.2, 'gps_lng': 106.8});
    final queued = (await o.list()).single;
    expect(queued.body, {'id': 's-1', 'gps_lat': -6.2, 'gps_lng': 106.8});
    o.dispose();
  });
}
