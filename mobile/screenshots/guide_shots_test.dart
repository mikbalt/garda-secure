// Tablet screenshots for the sales guide (docs/sales-guide/img/tab-*.png) in
// states the other screenshot files do not cover: a calm home screen, clock-in
// with a GPS fix, an incoming SOS on the home screen and an offline outbox.
// Not part of `flutter test test/`:
//   flutter test screenshots/guide_shots_test.dart --update-goldens
import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:provider/provider.dart';
import 'package:securepatrol/core/auth/auth_provider.dart';
import 'package:securepatrol/core/database/sync_service.dart';
import 'package:securepatrol/core/services/clock.dart';
import 'package:securepatrol/core/services/location_service.dart';
import 'package:securepatrol/core/services/photo_service.dart';
import 'package:securepatrol/features/attendance/clock_in_screen.dart';
import 'package:securepatrol/features/field_actions.dart';
import 'package:securepatrol/features/home/home_screen.dart';
import 'package:securepatrol/features/patrol/patrol_screen.dart';
import 'package:securepatrol/features/sync/outbox_screen.dart';
import 'package:securepatrol/shared/models/models.dart';
import 'package:securepatrol/theme/app_theme.dart';

import '../test/support/harness.dart';

/// A GPS fix at the gate of "Gudang Cikarang", without the platform plugin.
class _FixedLocation extends LocationService {
  @override
  Future<Fix?> current({Duration timeout = const Duration(seconds: 8)}) async => const Fix(-6.29431, 107.15302, 8);

  @override
  Fix? lastKnown() => const Fix(-6.29431, 107.15302, 8);
}

void main() {
  late Harness h;
  late FieldActions located;
  late DateTime now;
  late DateTime clockInAt;

  setUpAll(() async {
    await loadAppFonts();
    h = await Harness.create();
    h.seedSite();
    AppClock.setSiteZone('Asia/Jakarta');
    now = AppClock.now();
    // Round times read better: shift from the full hour three hours ago, next patrol on the next full hour.
    final hour = now.subtract(Duration(minutes: now.minute, seconds: now.second, milliseconds: now.millisecond, microseconds: now.microsecond));
    final shiftStart = hour.subtract(const Duration(hours: 3));
    h.session.guardVerified = true;
    h.session.guardStatus = GuardStatus(
      shift: Shift(id: 'sh-1', start: shiftStart, end: shiftStart.add(const Duration(hours: 8))),
      patrolsDue: [
        PatrolDue(routeId: 'r-1', routeName: 'Patroli Keliling Gudang', dueAt: hour.add(const Duration(hours: 1))),
        PatrolDue(routeId: 'r-2', routeName: 'Patroli Malam Perimeter', dueAt: hour.add(const Duration(hours: 10))),
      ],
    );
    h.outbox.lastSuccess = now.subtract(const Duration(minutes: 1));
    clockInAt = shiftStart.subtract(const Duration(minutes: 8));
    located = FieldActions(session: h.session, outbox: h.outbox, db: h.db, location: _FixedLocation(), photos: PhotoService());
  });

  /// Like `wrap`, but with [actions] instead of the harness's.
  Widget wrapWith(FieldActions actions, Widget child) => MultiProvider(
        providers: [
          ChangeNotifierProvider<DeviceSession>.value(value: h.session),
          ChangeNotifierProvider<Outbox>.value(value: h.outbox),
          ChangeNotifierProvider<FieldActions>.value(value: actions),
        ],
        child: MaterialApp(
          debugShowCheckedModeBanner: false,
          theme: AppTheme.light(),
          locale: const Locale('id'),
          supportedLocales: const [Locale('id'), Locale('en')],
          localizationsDelegates: GlobalMaterialLocalizations.delegates,
          home: child,
        ),
      );

  Future<void> shot(WidgetTester tester, Widget app, String name) async {
    await setSurface(tester, tabletSize);
    await tester.pumpWidget(app);
    await tester.pump(const Duration(milliseconds: 300));
    await tester.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 100)));
    await tester.pump(const Duration(milliseconds: 300));
    await expectLater(find.byType(MaterialApp), matchesGoldenFile('out/guide_$name.png'));
    await tester.pumpWidget(const SizedBox());
  }

  testWidgets('clock in with GPS fix', (t) async {
    located.clock = null;
    await shot(t, wrapWith(located, const ClockScreen(clockOut: false)), 'clock_in');
  });

  testWidgets('home, clocked in, next patrol due', (t) async {
    h.actions.clock = ClockState(attendanceId: 'a-1', clockInAt: clockInAt);
    await shot(t, wrap(h, const HomeScreen()), 'home');
  });

  testWidgets('route picker with schedules', (t) async {
    h.actions.patrol = null;
    await shot(t, wrap(h, const PatrolScreen()), 'routes');
  });

  testWidgets('home with an incoming SOS', (t) async {
    h.session.sosAlerts = [
      SosAlert(id: 'sos-1', guardId: 'g-4', guardName: 'Rina Wulandari', firedAt: now.subtract(const Duration(minutes: 3)), gpsLat: -6.29440, gpsLng: 107.15310),
    ];
    await shot(t, wrap(h, const HomeScreen()), 'home_sos');
    h.session.sosAlerts = const [];
  });

  testWidgets('outbox offline', (t) async {
    // Database work needs real async inside a widget test.
    await t.runAsync(() async {
      await h.outbox.enqueue(OutboxOp(id: 'o1', kind: 'clock_in', method: 'POST', path: '/x', guardId: 'g-1', guardToken: 't', label: 'Budi Santoso', body: const {}), send: false);
      await h.outbox.enqueue(OutboxOp(id: 'o2', kind: 'scan', method: 'POST', path: '/x', guardId: 'g-1', guardToken: 't', label: 'Gerbang Utama', body: const {}), send: false);
      await h.outbox.enqueue(OutboxOp(id: 'o3', kind: 'incident', method: 'POST', path: '/x', guardId: 'g-1', guardToken: 't', label: 'Penyusupan', body: const {}), send: false);
      await h.db.db.update('outbox', {'last_code': 'network', 'last_error': 'offline', 'attempts': 2});
      await h.outbox.refreshCounts();
    });
    h.outbox.online = false;
    h.outbox.lastSuccess = now.subtract(const Duration(minutes: 12));
    await shot(t, wrap(h, const OutboxScreen()), 'outbox');
  });
}
