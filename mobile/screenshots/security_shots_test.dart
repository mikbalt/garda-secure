// Screenshots of the hardening UI (audit MOB-*). Not part of `flutter test`:
//   flutter test screenshots/security_shots_test.dart --update-goldens
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:securepatrol/core/auth/auth_provider.dart';
import 'package:securepatrol/core/database/sync_service.dart';
import 'package:securepatrol/core/services/clock.dart';
import 'package:securepatrol/features/auth/pin_screen.dart';
import 'package:securepatrol/features/auth/supervisor_gate.dart';
import 'package:securepatrol/features/device/device_info_screen.dart';
import 'package:securepatrol/features/home/home_screen.dart';
import 'package:securepatrol/features/patrol/patrol_screen.dart';
import 'package:securepatrol/features/sync/outbox_screen.dart';
import 'package:securepatrol/shared/models/models.dart';

import '../test/support/harness.dart';

void main() {
  late Harness h;
  setUpAll(() async {
    await loadAppFonts();
    h = await Harness.create();
    h.seedSite();
    AppClock.setSiteZone('Asia/Jakarta');
    final now = AppClock.now();
    h.session.guardStatus = GuardStatus(patrolsDue: [
      PatrolDue(routeId: 'r-2', routeName: 'Patroli Malam Perimeter', dueAt: now.subtract(const Duration(minutes: 20)), status: 'pending'),
      PatrolDue(routeId: 'r-1', routeName: 'Patroli Keliling Gudang', dueAt: now.add(const Duration(hours: 2))),
    ]);
    await h.outbox.enqueue(OutboxOp(id: 'f1', kind: 'scan', method: 'POST', path: '/x', guardId: 'g-1', label: 'Gerbang Utama', body: const {}), send: false);
    await h.outbox.enqueue(OutboxOp(id: 'p1', kind: 'incident', method: 'POST', path: '/x', guardId: 'g-1', guardToken: 't', label: 'Penyusupan', body: const {}), send: false);
    await h.db.db.update('outbox', {'status': 'failed', 'last_code': 'session_ended', 'last_error': 'session already ended'}, where: "id = 'f1'");
    await h.db.db.update('outbox', {'last_code': 'network', 'last_error': 'offline', 'attempts': 3}, where: "id = 'p1'");
    await h.outbox.refreshCounts();
  });

  Future<void> shot(WidgetTester tester, Widget w, String name, {Size size = tabletSize, Locale locale = const Locale('id'), Future<void> Function()? act}) async {
    await setSurface(tester, size);
    await tester.pumpWidget(wrap(h, w, locale: locale));
    await tester.pump(const Duration(milliseconds: 300));
    await tester.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 100)));
    await tester.pump(const Duration(milliseconds: 300));
    if (act != null) {
      await act();
      await tester.pump(const Duration(milliseconds: 400));
    }
    await expectLater(find.byType(MaterialApp), matchesGoldenFile('out/sec_$name.png'));
    await tester.pumpWidget(const SizedBox());
  }

  testWidgets('home with notices (verified)', (t) async {
    h.session.guardVerified = true;
    AppClock.offset = const Duration(minutes: 7);
    await shot(t, const HomeScreen(), 'home');
    AppClock.offset = Duration.zero;
  });

  testWidgets('home phone, offline unlock', (t) async {
    h.session.guardVerified = false;
    await shot(t, const HomeScreen(), 'home_phone', size: phoneSize, locale: const Locale('en'));
  });

  testWidgets('route picker with schedules', (t) => shot(t, const PatrolScreen(), 'routes_due'));

  testWidgets('outbox with a rejected item', (t) => shot(t, const OutboxScreen(), 'outbox'));

  testWidgets('device info', (t) => shot(t, const DeviceInfoScreen(), 'device'));

  testWidgets('supervisor approval', (t) => shot(t, const Scaffold(body: SupervisorGateDialog()), 'gate'));

  testWidgets('lock screen: notice + PIN panel with SOS', (t) async {
    h.session.phase = Phase.locked;
    h.session.lockNotice = {'code': 'wrongPin', 'name': 'Agus Pratama'};
    await shot(t, const PinScreen(), 'lock_sos', act: () async {
      await t.tap(find.text('Dedi Kurniawan'));
    });
    h.session.lockNotice = null;
  });

  testWidgets('lock screen phone: PIN page with SOS', (t) async {
    h.session.phase = Phase.locked;
    await shot(t, const PinScreen(), 'lock_sos_phone', size: phoneSize, act: () async {
      await t.tap(find.text('Dedi Kurniawan'));
      await t.pump(const Duration(milliseconds: 500));
    });
  });
}
