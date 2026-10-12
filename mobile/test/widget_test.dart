import 'package:flutter_test/flutter_test.dart';
import 'package:securepatrol/config/env.dart';
import 'package:securepatrol/core/auth/keycloak_service.dart';
import 'package:securepatrol/core/database/sync_service.dart';
import 'package:securepatrol/core/services/pin_verifier.dart';
import 'package:securepatrol/features/patrol/progress.dart';
import 'package:securepatrol/l10n/strings.dart';
import 'package:securepatrol/shared/models/models.dart';

import 'support/harness.dart';

void main() {
  group('strings', () {
    test('Indonesian and English have the same keys', () {
      expect(S.allId.keys.toSet().difference(S.allEn.keys.toSet()), isEmpty, reason: 'missing in English');
      expect(S.allEn.keys.toSet().difference(S.allId.keys.toSet()), isEmpty, reason: 'missing in Indonesian');
    });
    test('placeholders are replaced', () {
      expect(const S('id').t('pat.progress', {'a': 2, 'n': 5}), '2 dari 5 titik');
      expect(const S('en').t('pat.progress', {'a': 2, 'n': 5}), '2 of 5 checkpoints');
    });
  });

  group('server config', () {
    test('normalises the address and derives endpoints', () {
      final url = ServerConfig.normalizeUrl(' patrol.example.co.id/ ');
      expect(url, 'https://patrol.example.co.id');
      final c = ServerConfig(serverUrl: url, realm: 'client1');
      expect(c.apiBase, 'https://patrol.example.co.id/api/v1');
      expect(c.tokenEndpoint, 'https://patrol.example.co.id/auth/realms/client1/protocol/openid-connect/token');
    });
  });

  group('PIN', () {
    // Hash produced by the Go backend (golang.org/x/crypto/bcrypt, cost 12).
    const goHash = r'$2a$12$lFmYGucJNe.5H6xUWs6kiOPN2YrL89zSnPhr7fPwNpxA.G5zOIwc.';
    test('accepts the right PIN and rejects others', () {
      expect(PinVerifier.verifySync('4826', goHash), isTrue);
      expect(PinVerifier.verifySync('4827', goHash), isFalse);
      expect(PinVerifier.verifySync('4826', 'not-a-hash'), isFalse);
    });
    test('locks out after repeated failures, escalating, and survives a restart', () {
      Map<String, dynamic>? saved;
      final a = PinAttempts(maxAttempts: 3, onChanged: (s) => saved = s);
      final t0 = DateTime(2026, 10, 9, 12);
      expect(a.fail('g', t0), 2);
      expect(a.fail('g', t0), 1);
      expect(a.fail('g', t0), 0);
      expect(a.waitSeconds('g', t0.add(const Duration(seconds: 10))), 20);
      expect(a.waitSeconds('g', t0.add(const Duration(seconds: 31))), 0);
      // The next wrong PIN locks at once, for longer (2 min).
      final t1 = t0.add(const Duration(seconds: 40));
      expect(a.fail('g', t1), 0);
      expect(a.waitSeconds('g', t1.add(const Duration(seconds: 100))), 20);
      final t2 = t1.add(const Duration(minutes: 3));
      expect(a.fail('g', t2), 0);
      expect(a.waitSeconds('g', t2.add(const Duration(minutes: 9))), 60, reason: 'third step: 10 min');

      // Restart: the state is restored from what was persisted.
      final b = PinAttempts(maxAttempts: 3)..load(saved);
      expect(b.waitSeconds('g', t2.add(const Duration(minutes: 9))), 60);
      expect(b.failureCounts, {'g': 5});
      b.success('g');
      expect(b.waitSeconds('g', t2), 0);
      expect(b.failureCounts, isEmpty);
    });

    test('a server lockout (423) is applied locally', () {
      final a = PinAttempts();
      final t0 = DateTime(2026, 10, 9, 12);
      a.lockFor('g', const Duration(seconds: 90), t0);
      expect(a.waitSeconds('g', t0), 90);
    });
  });

  test('device role is read from the token', () {
    // header.payload.signature with payload {"roles":["device"],"realm_access":{"roles":["offline_access"]}}
    const jwt = 'eyJhbGciOiJSUzI1NiJ9.eyJyb2xlcyI6WyJkZXZpY2UiXSwicmVhbG1fYWNjZXNzIjp7InJvbGVzIjpbIm9mZmxpbmVfYWNjZXNzIl19fQ.sig';
    expect(KeycloakService.rolesOf(jwt), containsAll(['device', 'offline_access']));
    expect(KeycloakService.rolesOf('garbage'), isEmpty);
  });

  group('patrol progress mirrors the server SLA rule', () {
    const route = PatrolRoute(id: 'r', name: 'R', checkpoints: [
      Checkpoint(id: 'a', name: 'A', qrCode: 'QA', slaMinutes: 10, orderIndex: 0),
      Checkpoint(id: 'b', name: 'B', qrCode: 'QB', slaMinutes: 15, orderIndex: 1),
      Checkpoint(id: 'c', name: 'C', qrCode: 'QC', orderIndex: 2),
      Checkpoint(id: 'd', name: 'D', qrCode: 'QD', slaMinutes: 5, orderIndex: 3),
    ]);
    final start = DateTime.utc(2026, 10, 7, 20);
    final scans = [
      LocalScan(checkpointId: 'a', scannedAt: start.add(const Duration(minutes: 8))),
      LocalScan(checkpointId: 'b', scannedAt: start.add(const Duration(minutes: 30))),
    ];

    test('statuses and next checkpoint', () {
      final p = computeProgress(route, start, scans, start.add(const Duration(minutes: 45)));
      expect(p.items.map((i) => i.status).toList(), [CpStatus.onTime, CpStatus.late, CpStatus.pending, CpStatus.overdue]);
      expect(p.scanned, 2);
      expect(p.nextIndex, 2);
      expect(p.items[3].dueAt, start.add(const Duration(minutes: 35)));
    });
    test('ended sessions mark unscanned checkpoints missed', () {
      final p = computeProgress(route, start, scans, start.add(const Duration(minutes: 45)), ended: true);
      expect(p.items[2].status, CpStatus.missed);
      expect(p.nextIndex, isNull);
    });
    test('lookup by QR payload', () {
      expect(route.byCode(' QC ')?.id, 'c');
      expect(route.byCode('nope'), isNull);
    });
  });

  group('outbox', () {
    late Harness h;
    setUp(() async => h = await Harness.create());
    tearDown(() async => h.dispose());

    test('keeps strict order and SOS first', () async {
      final t = DateTime.utc(2026, 10, 9, 10);
      await h.outbox.enqueue(OutboxOp(id: '1', kind: 'session_start', method: 'POST', path: '/x', body: const {}, createdAt: t), send: false);
      await h.outbox.enqueue(OutboxOp(id: '2', kind: 'scan', method: 'POST', path: '/x', body: const {}, createdAt: t.add(const Duration(seconds: 1))), send: false);
      await h.outbox.enqueue(OutboxOp(id: '3', kind: 'sos', method: 'POST', path: '/sos', body: const {}, priority: 0, createdAt: t.add(const Duration(seconds: 2))), send: false);
      final ids = (await h.outbox.list()).map((o) => o.id).toList();
      expect(ids, ['3', '1', '2']);
      await h.outbox.refreshCounts();
      expect(h.outbox.pending, 3);
      expect(h.outbox.sosPending, isTrue);
    });
    test('backoff grows and SOS retries every 10 s', () {
      expect(Outbox.backoff('sos', 5), const Duration(seconds: 10));
      expect(Outbox.backoff('scan', 1), const Duration(seconds: 5));
      expect(Outbox.backoff('scan', 3), const Duration(seconds: 30));
      expect(Outbox.backoff('scan', 99), const Duration(seconds: 600));
    });
    test('field actions queue idempotent requests with client IDs', () async {
      h.seedSite();
      await h.actions.startPatrol(h.session.routes.first);
      final cp = await h.actions.scan('SP-DOCK-2');
      expect(cp?.id, 'c-2');
      expect(await h.actions.scan('SP-DOCK-2'), isNotNull, reason: 'repeat scan is ignored');
      expect(await h.actions.scan('UNKNOWN'), isNull);
      final ops = await h.outbox.list();
      expect(ops.map((o) => o.kind), ['session_start', 'scan']);
      final sessionId = ops.first.body['id'];
      expect(ops[1].path, '/patrol/sessions/$sessionId/scans');
      expect(ops[1].guardId, 'g-1');
      expect(ops[1].body['id'], isA<String>());
      expect(h.actions.patrol!.progress(DateTime.now().toUtc()).scanned, 1);
    });
  });
}
