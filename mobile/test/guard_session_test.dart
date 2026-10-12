import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:securepatrol/core/api/api_client.dart';
import 'package:securepatrol/core/auth/auth_provider.dart';
import 'package:securepatrol/core/database/local_db.dart';
import 'package:securepatrol/core/database/sync_service.dart';
import 'package:securepatrol/core/services/clock.dart';
import 'package:securepatrol/core/services/location_service.dart';
import 'package:securepatrol/core/storage/secure_store.dart';
import 'package:securepatrol/features/auth/pin_screen.dart';
import 'package:securepatrol/shared/models/models.dart';
import 'package:securepatrol/shared/widgets/sos_button.dart';

import 'support/fake_api.dart';
import 'support/harness.dart';

const _pin = '4826'; // matches Budi's (g-1) hash in the harness

Future<Object?> Function(Call) server({Object? Function(Call c)? guardSessions, Object? Function(Call c)? other}) => (c) async {
      if (c.path == '/device/guard-sessions' && guardSessions != null) {
        final r = guardSessions(c);
        if (r is ApiException) throw r;
        return r;
      }
      final r = other?.call(c);
      if (r is ApiException) throw r;
      if (r != null) return r;
      throw const ApiException(network: true, message: 'offline');
    };

Map<String, dynamic> token(String t) => {'token': t, 'guard_id': 'g-1', 'expires_at': AppClock.now().add(const Duration(hours: 8)).toIso8601String()};

void main() {
  late Harness h;
  late RosterGuard budi;
  setUp(() async {
    h = await Harness.create();
    h.seedSite(unlocked: false);
    budi = h.session.roster.first;
  });
  tearDown(() async {
    await h.dispose();
    AppClock.reset();
  });

  group('guard sessions (D-035)', () {
    test('online: the server verifies, the token is cached and attached to queued actions', () async {
      h.api.handler = server(guardSessions: (c) => token('tok-1'), other: (c) => const <String, dynamic>{});
      final r = await h.session.unlockWithPin(budi, _pin);
      expect(r.status, UnlockStatus.unlocked);
      expect(h.session.guardVerified, isTrue);
      expect(h.session.guardToken, 'tok-1');
      expect(h.store.data[SecureKeys.guardToken('g-1')], contains('tok-1'));
      expect(h.api.calls.first.json, {'guard_id': 'g-1', 'pin': _pin});

      await h.actions.reportIncident(type: 'theft', severity: 'low', fix: const Fix(-6.2, 106.8, 12));
      final op = (await h.outbox.list()).single;
      expect(op.guardToken, 'tok-1');
      expect(op.body['gps_lat'], -6.2, reason: 'incident GPS (D-049)');
      expect(op.body['gps_lng'], 106.8);
      await h.outbox.process();
      expect(h.api.calls.where((c) => c.path == '/incidents').single.guardToken, 'tok-1');
    });

    test('the server is authoritative: wrong PIN although the cached hash matches', () async {
      h.api.handler = server(guardSessions: (c) => const ApiException(status: 401, code: 'invalid_pin', message: 'no', details: {'attempts_left': 2}));
      final r = await h.session.unlockWithPin(budi, _pin);
      expect(r.status, UnlockStatus.wrongPin);
      expect(r.attemptsLeft, 2);
      expect(h.session.phase, Phase.locked);
      await Future<void>.delayed(const Duration(milliseconds: 50));
      expect(h.api.paths, contains('/device/roster'), reason: 'stale hash: roster resynced');
    });

    test('423 pin_locked locks the PIN pad for retry_after', () async {
      h.api.handler = server(guardSessions: (c) => const ApiException(status: 423, code: 'pin_locked', message: 'locked', retryAfter: Duration(seconds: 120)));
      final r = await h.session.unlockWithPin(budi, _pin);
      expect(r.status, UnlockStatus.locked);
      expect(r.waitSeconds, inInclusiveRange(119, 120));
      expect((await h.session.unlockWithPin(budi, _pin)).status, UnlockStatus.locked, reason: 'no server call while locked');
    });

    test('403 / 409 from the server refuse the unlock', () async {
      h.api.handler = server(guardSessions: (c) => const ApiException(status: 403, code: 'guard_not_allowed', message: 'x'));
      expect((await h.session.unlockWithPin(budi, _pin)).status, UnlockStatus.notAllowed);
      h.api.handler = server(guardSessions: (c) => const ApiException(status: 409, code: 'pin_not_set', message: 'x'));
      expect((await h.session.unlockWithPin(budi, _pin)).status, UnlockStatus.pinNotSet);
      expect(h.session.phase, Phase.locked);
    });

    test('offline: the local check unlocks, unverified and without a token', () async {
      await h.store.write(SecureKeys.guardToken('g-1'), '{"token":"old","expires_at":"2099-01-01T00:00:00Z"}');
      final bad = await h.session.unlockWithPin(budi, '1111');
      expect(bad.status, UnlockStatus.wrongPin);
      expect(bad.attemptsLeft, 4);
      final r = await h.session.unlockWithPin(budi, _pin);
      expect(r.status, UnlockStatus.unlocked);
      expect(h.session.guardVerified, isFalse);
      expect(h.session.guardToken, isNull);
      expect(h.store.data[SecureKeys.guardToken('g-1')], isNull, reason: 'an older token is dropped');
      await h.actions.reportDefect(type: 'Lampu mati');
      expect((await h.outbox.list()).single.guardToken, isNull);
    });

    test('a slow server that rejects later locks the tablet again', () async {
      h.session.serverCheckWait = const Duration(milliseconds: 50);
      h.api.handler = (c) async {
        if (c.path != '/device/guard-sessions') throw const ApiException(network: true, message: 'offline');
        await Future<void>.delayed(const Duration(milliseconds: 400));
        throw const ApiException(status: 401, code: 'invalid_pin', message: 'no', details: {'attempts_left': 4});
      };
      final r = await h.session.unlockWithPin(budi, _pin);
      expect(r.status, UnlockStatus.unlocked);
      expect(h.session.guardVerified, isFalse);
      await Future<void>.delayed(const Duration(milliseconds: 600));
      expect(h.session.phase, Phase.locked);
      expect(h.session.lockNotice?['name'], 'Budi Santoso');
    });

    test('a slow server that confirms later marks the guard verified', () async {
      h.session.serverCheckWait = const Duration(milliseconds: 50);
      h.api.handler = (c) async {
        if (c.path != '/device/guard-sessions') return const <String, dynamic>{};
        await Future<void>.delayed(const Duration(milliseconds: 300));
        return token('tok-late');
      };
      await h.session.unlockWithPin(budi, _pin);
      expect(h.session.guardVerified, isFalse);
      await Future<void>.delayed(const Duration(milliseconds: 500));
      expect(h.session.guardVerified, isTrue);
      expect(h.session.guardToken, 'tok-late');
    });

    test('a rejected token is dropped and the guard shown as unverified', () async {
      h.api.handler = server(guardSessions: (c) => token('tok-1'), other: (c) => const <String, dynamic>{});
      await h.session.unlockWithPin(budi, _pin);
      h.session.onGuardTokenInvalid('g-1');
      await Future<void>.delayed(Duration.zero);
      expect(h.session.guardVerified, isFalse);
      expect(h.session.guardToken, isNull);
      expect(h.store.data[SecureKeys.guardToken('g-1')], isNull);
    });
  });

  group('MOB-6', () {
    test('no unlock after 7 days without a roster sync (SOS still works)', () async {
      h.session.referenceSyncedAt = AppClock.now().subtract(const Duration(days: 8));
      final r = await h.session.unlockWithPin(budi, _pin);
      expect(r.status, UnlockStatus.rosterStale);
      expect(h.session.phase, Phase.locked);
      await h.actions.sos(as: h.session.roster[1]);
      final sos = (await h.outbox.list()).single;
      expect(sos.kind, 'sos');
      expect(sos.guardId, 'g-2');
    });

    test('the PIN lockout survives an app restart', () async {
      for (var i = 0; i < 5; i++) {
        await h.session.unlockWithPin(budi, '0000');
      }
      expect(h.session.pinAttempts.waitSeconds('g-1'), greaterThan(0));
      await Future<void>.delayed(const Duration(milliseconds: 50));
      final restarted = DeviceSession(store: h.store, auth: h.session.auth, db: h.db, api: h.api);
      restarted.pinAttempts.load(await h.db.getMap(CacheKeys.pinLockout));
      expect(restarted.pinAttempts.waitSeconds('g-1'), greaterThan(0));
    });

    test('a revoked device drops roster, routes and tokens but keeps unsent items', () async {
      await h.db.put(CacheKeys.roster, h.session.roster.map((g) => g.toJson()).toList());
      await h.store.write(SecureKeys.guardToken('g-1'), '{}');
      await h.outbox.enqueue(OutboxOp(id: '1', kind: 'incident', method: 'POST', path: '/x', body: const {}), send: false);
      h.api.handler = server(other: (c) => c.path == '/device/me' ? {'device': {'id': 'dev-1', 'name': 'T', 'status': 'revoked', 'site_id': 'site-1'}} : null);
      await h.session.refreshDevice();
      expect(h.session.phase, Phase.revoked);
      expect(h.session.roster, isEmpty);
      expect(h.session.routes, isEmpty);
      expect(await h.db.get(CacheKeys.roster), isNull);
      expect(h.store.data.keys.where((k) => k.startsWith('sp.gt.')), isEmpty);
      expect(await h.db.outboxCount(), 1);
    });
  });

  group('MOB-2', () {
    test('sign-out is refused while items are queued; never wipes them', () async {
      await h.outbox.enqueue(OutboxOp(id: '1', kind: 'incident', method: 'POST', path: '/x', body: const {}), send: false);
      await expectLater(h.session.signOutDevice(), throwsA(isA<OutboxNotEmpty>()));
      expect(await h.db.outboxCount(), 1);
      await expectLater(h.session.editServer(), throwsA(isA<OutboxNotEmpty>()));
      await h.outbox.discard('1');
      await h.session.signOutDevice();
      expect(h.session.phase, Phase.login);
      expect(h.session.roster, isEmpty);
    });
  });

  test('event times use the server clock (MOB-4)', () async {
    AppClock.offset = const Duration(minutes: 10);
    await h.session.unlock(budi);
    await h.actions.clockIn();
    final at = DateTime.parse((await h.outbox.list()).single.body['clock_in_at'] as String);
    expect(at.difference(DateTime.now().toUtc()).inMinutes, inInclusiveRange(9, 10));
  });

  testWidgets('lock screen: tap your name, hold SOS — no PIN (MOB-3)', (tester) async {
    await setSurface(tester, tabletSize);
    await tester.pumpWidget(wrap(h, const PinScreen()));
    await tester.pump();
    await tester.tap(find.text('Agus Pratama'));
    await tester.pump();
    final sos = find.byType(SosButton);
    expect(sos, findsOneWidget);
    final g = await tester.startGesture(tester.getCenter(sos));
    for (var i = 0; i < 24; i++) {
      await tester.pump(const Duration(milliseconds: 100)); // the ring fills in 2 s
    }
    await g.up();
    for (var i = 0; i < 10; i++) {
      await tester.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 50)));
      await tester.pump(const Duration(milliseconds: 50));
    }
    final ops = (await tester.runAsync(h.outbox.list))!;
    expect(ops.single.kind, 'sos');
    expect(ops.single.guardId, 'g-2');
    expect(ops.single.guardToken, isNull, reason: 'no PIN: sent unverified');
    expect(h.actions.sosOutstanding, isNotEmpty, reason: 'the delivery banner tracks it');
    expect(h.session.phase, Phase.locked);
    await tester.pumpWidget(const SizedBox());
  });
}
