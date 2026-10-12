import 'dart:async';
import 'dart:io';

import 'package:connectivity_plus/connectivity_plus.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:securepatrol/core/api/api_client.dart';
import 'package:securepatrol/core/database/local_db.dart';
import 'package:securepatrol/core/database/sync_service.dart';
import 'package:sqflite_common_ffi/sqflite_ffi.dart';

import 'support/fake_api.dart';

OutboxOp op(String id, String kind, {int priority = 1, Map<String, dynamic>? body, String? token, List<OutboxPhoto> photos = const [], String guard = 'g-1'}) => OutboxOp(
      id: id,
      kind: kind,
      method: 'POST',
      path: '/$kind',
      guardId: guard,
      guardToken: token,
      priority: priority,
      body: body ?? {'id': id},
      photos: photos,
    );

ApiException err(int status, String code, {Duration? retryAfter}) => ApiException(status: status, code: code, message: code, retryAfter: retryAfter);

void main() {
  late LocalDb db;
  setUp(() async {
    sqfliteFfiInit();
    db = await LocalDb.open(factory: databaseFactoryFfi, path: inMemoryDatabasePath);
  });
  tearDown(() => db.db.close());

  Outbox outboxWith(FakeApi api, {Stream<List<ConnectivityResult>>? connectivity, void Function(String)? onDeviceDenied, void Function(String)? onGuardTokenInvalid, int maxAttempts = 50}) => Outbox(
        db,
        api,
        connectivity: connectivity ?? const Stream<List<ConnectivityResult>>.empty(),
        tick: const Duration(hours: 1),
        onDeviceDenied: onDeviceDenied,
        onGuardTokenInvalid: onGuardTokenInvalid,
        maxAttempts: maxAttempts,
      );

  Future<OutboxOp> only(Outbox o) async => (await o.list()).single;

  test('an SOS rejected by the server is never parked', () async {
    final api = FakeApi(script: [err(403, 'guard_not_allowed')]);
    final o = outboxWith(api);
    await o.enqueue(op('s-1', 'sos', priority: 0));
    await o.process();
    expect(o.failed, 0);
    expect(o.sosPending, isTrue);
    expect(o.sosRejected, isTrue);
    o.dispose();
  });

  test('an SOS rejected for clock skew is resent at once without the client time', () async {
    final api = FakeApi(script: [err(400, 'timestamp_out_of_range')]);
    final o = outboxWith(api);
    await o.enqueue(op('s-1', 'sos', priority: 0, body: {'id': 's-1', 'fired_at': '2020-01-01T00:00:00Z'}));
    await o.process();
    expect(api.calls, hasLength(2));
    expect(api.calls.last.json, {'id': 's-1'});
    expect(o.sosPending, isFalse);
    expect(o.sosRejected, isFalse);
    o.dispose();
  });

  test('other rejected operations are parked with their code and the queue moves on', () async {
    final api = FakeApi(script: [err(400, 'no_shift')]);
    final o = outboxWith(api);
    await o.enqueue(op('a', 'clock_in'), send: false);
    await o.enqueue(op('b', 'incident'), send: false);
    await o.process();
    expect(o.failed, 1);
    expect(o.pending, 0);
    expect(api.paths, ['/clock_in', '/incident']);
    expect((await only(o)).lastCode, 'no_shift');
    o.dispose();
  });

  test('id_conflict is a terminal failure', () async {
    final api = FakeApi(script: [err(409, 'id_conflict')]);
    final o = outboxWith(api);
    await o.enqueue(op('a', 'incident'), send: false);
    await o.process();
    final parked = await only(o);
    expect(parked.status, 'failed');
    expect(parked.lastCode, 'id_conflict');
    expect(api.calls, hasLength(1));
    o.dispose();
  });

  test('coming back online retries at once instead of waiting out the backoff', () async {
    final conn = StreamController<List<ConnectivityResult>>();
    final api = FakeApi(script: [for (var i = 0; i < 6; i++) const ApiException(network: true, message: 'offline')]);
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
    final o = outboxWith(FakeApi());
    await o.enqueue(op('s-1', 'sos', priority: 0), send: false);
    await o.amendBody('s-1', {'gps_lat': -6.2, 'gps_lng': 106.8});
    final queued = await only(o);
    expect(queued.body, {'id': 's-1', 'gps_lat': -6.2, 'gps_lng': 106.8});
    o.dispose();
  });

  group('MOB-5', () {
    test('a poison item (server errors) is parked after the attempt cap; network errors never park', () async {
      final api = FakeApi(handler: (c) async => throw err(500, 'internal'));
      final o = outboxWith(api, maxAttempts: 3);
      await o.enqueue(op('a', 'incident'), send: false);
      await o.enqueue(op('b', 'defect'), send: false);
      await o.flush();
      await o.flush();
      expect(o.failed, 0, reason: 'still retrying');
      await o.flush(); // third failure: parked, then the next item is tried
      final items = await o.list();
      expect(items.first.status, 'failed');
      expect(items.first.lastCode, 'retry_limit');
      expect(api.paths.last, '/defect');

      api.handler = (c) async => throw const ApiException(network: true, message: 'offline');
      for (var i = 0; i < 6; i++) {
        await o.flush();
      }
      expect((await o.list()).where((x) => x.id == 'b').single.status, 'pending');
      o.dispose();
    });

    test('an SOS is never capped', () async {
      final api = FakeApi(handler: (c) async => throw err(503, 'unavailable'));
      final o = outboxWith(api, maxAttempts: 2);
      await o.enqueue(op('s', 'sos', priority: 0), send: false);
      for (var i = 0; i < 4; i++) {
        await o.flush();
      }
      expect(o.failed, 0);
      expect(o.sosPending, isTrue);
      o.dispose();
    });

    test('replays the server reports as already done count as sent', () async {
      final api = FakeApi(script: [err(409, 'not_clocked_in'), err(409, 'session_ended')]);
      final o = outboxWith(api);
      await o.enqueue(op('a', 'clock_out'), send: false);
      await o.enqueue(op('b', 'session_end'), send: false);
      await o.process();
      expect(o.pending + o.failed, 0);
      o.dispose();
    });

    test('a clock-out is not "already done" when our own clock-in failed', () async {
      final api = FakeApi(script: [err(400, 'no_shift'), err(409, 'not_clocked_in')]);
      final o = outboxWith(api);
      await o.enqueue(op('a', 'clock_in'), send: false);
      await o.enqueue(op('b', 'clock_out'), send: false);
      await o.process();
      expect(o.failed, 2);
      o.dispose();
    });

    test('already_clocked_in: later clock-outs close the server record instead of our ID', () async {
      final api = FakeApi(script: [err(409, 'already_clocked_in')]);
      final o = outboxWith(api);
      await o.enqueue(op('a', 'clock_in', body: {'id': 'att-1'}), send: false);
      await o.enqueue(op('b', 'clock_out', body: {'attendance_id': 'att-1', 'clock_out_at': 'x'}), send: false);
      await o.process();
      expect(o.pending + o.failed, 0);
      expect(api.calls.last.json, {'clock_out_at': 'x'});
      o.dispose();
    });

    test('device-level 403 pauses the whole queue without failing items', () async {
      final denied = <String>[];
      final api = FakeApi(script: [err(403, 'device_revoked')]);
      final o = outboxWith(api, onDeviceDenied: denied.add);
      await o.enqueue(op('a', 'incident'), send: false);
      await o.enqueue(op('b', 'defect'), send: false);
      await o.process();
      expect(denied, ['device_revoked']);
      expect(o.pauseCode, 'device_revoked');
      expect(o.failed, 0);
      expect(o.pending, 2);
      final items = await o.list();
      expect(items.every((x) => x.attempts == 0), isTrue, reason: 'items untouched');
      await o.process();
      expect(api.calls, hasLength(1), reason: 'paused: nothing sent');
      o.resume();
      await Future<void>.delayed(const Duration(milliseconds: 50));
      await o.process();
      expect(o.pending, 0);
      o.dispose();
    });

    test('429 waits for Retry-After', () async {
      final api = FakeApi(script: [err(429, 'rate_limited', retryAfter: const Duration(seconds: 90))]);
      final o = outboxWith(api);
      await o.enqueue(op('a', 'incident'), send: false);
      final before = DateTime.now().toUtc();
      await o.process();
      final item = await only(o);
      expect(item.status, 'pending');
      expect(item.nextAt.difference(before).inSeconds, inInclusiveRange(89, 91));
      await o.process();
      expect(api.calls, hasLength(1), reason: 'not before Retry-After');
      o.dispose();
    });

    test('403 guard_token_invalid: the token is dropped and the item resent at once', () async {
      final invalid = <String>[];
      final api = FakeApi(handler: (c) async {
        if (c.guardToken != null) throw err(403, 'guard_token_invalid');
        return const {};
      });
      final o = outboxWith(api, onGuardTokenInvalid: invalid.add);
      await o.enqueue(op('a', 'incident', token: 'tok'), send: false);
      await o.process();
      expect(api.calls.map((c) => c.guardToken), ['tok', null]);
      expect(invalid, ['g-1']);
      expect(o.pending + o.failed, 0);
      o.dispose();
    });

    test('a missing photo file parks the item instead of pretending it uploaded', () async {
      final api = FakeApi();
      final o = outboxWith(api);
      await o.enqueue(op('a', 'incident', photos: [const OutboxPhoto(id: 'p', kind: 'incident', path: '/nonexistent/p.jpg')]), send: false);
      await o.process();
      final item = await only(o);
      expect(item.status, 'failed');
      expect(item.lastCode, 'photo_missing');
      expect(api.calls, isEmpty);
      o.dispose();
    });

    test('media_exists on upload counts as uploaded', () async {
      final dir = await Directory.systemTemp.createTemp('sp_photo');
      final f = File('${dir.path}/p.jpg')..writeAsBytesSync([0xFF, 0xD8, 0xFF, 0xD9]);
      final api = FakeApi(script: [err(409, 'media_exists'), null]);
      final o = outboxWith(api);
      await o.enqueue(op('a', 'incident', photos: [OutboxPhoto(id: 'p', kind: 'incident', path: f.path)]), send: false);
      await o.process();
      expect(api.paths, ['/media', '/incident']);
      expect(o.pending + o.failed, 0);
      expect(f.existsSync(), isFalse, reason: 'sent: file freed');
      await dir.delete(recursive: true);
      o.dispose();
    });

    test('order follows the local sequence, not the (device-clock) creation time', () async {
      final o = outboxWith(FakeApi());
      final t = DateTime.utc(2026, 10, 9, 10);
      await o.enqueue(OutboxOp(id: '1', kind: 'session_start', method: 'POST', path: '/x', body: const {}, createdAt: t), send: false);
      // The clock was corrected backwards before the next action.
      await o.enqueue(OutboxOp(id: '2', kind: 'scan', method: 'POST', path: '/x', body: const {}, createdAt: t.subtract(const Duration(hours: 2))), send: false);
      expect((await o.list()).map((x) => x.id), ['1', '2']);
      o.dispose();
    });
  });
}
