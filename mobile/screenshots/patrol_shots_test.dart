// Screenshots of the patrol screens (not part of `flutter test test/`): run
//   flutter test screenshots/patrol_shots_test.dart --update-goldens
// and look at screenshots/out/patrol_*.png.
import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:securepatrol/features/field_actions.dart';
import 'package:securepatrol/features/patrol/patrol_screen.dart';
import 'package:securepatrol/features/patrol/progress.dart';
import 'package:securepatrol/features/patrol/scanner_screen.dart';

import '../test/support/harness.dart';

void main() {
  late Harness h;
  setUpAll(() async {
    await loadAppFonts();
    h = await Harness.create();
    h.seedSite();
  });

  /// Route "Patroli Keliling Gudang" 45 min in: gate on time, dock late,
  /// warehouse 5 min overdue, the rest pending.
  ActivePatrol midway({bool all = false}) {
    final now = DateTime.now().toUtc();
    final start = now.subtract(const Duration(minutes: 45));
    return ActivePatrol(
      sessionId: 's-1',
      route: h.session.routes.first,
      startedAt: start,
      scans: [
        LocalScan(checkpointId: 'c-1', scannedAt: now.subtract(const Duration(minutes: 37))),
        LocalScan(checkpointId: 'c-2', scannedAt: now.subtract(const Duration(minutes: 20)), method: all ? 'qr' : 'manual'),
        if (all) ...[
          LocalScan(checkpointId: 'c-3', scannedAt: now.subtract(const Duration(minutes: 12))),
          LocalScan(checkpointId: 'c-4', scannedAt: now.subtract(const Duration(minutes: 6))),
          LocalScan(checkpointId: 'c-5', scannedAt: now.subtract(const Duration(minutes: 1))),
        ],
      ],
    );
  }

  testWidgets('route picker tablet', (tester) async {
    h.actions.patrol = null;
    await setSurface(tester, tabletSize);
    await tester.pumpWidget(wrap(h, const PatrolScreen()));
    await tester.pump();
    await expectLater(find.byType(MaterialApp), matchesGoldenFile('out/patrol_routes_tablet.png'));
  });

  testWidgets('route picker phone', (tester) async {
    h.actions.patrol = null;
    await setSurface(tester, phoneSize);
    await tester.pumpWidget(wrap(h, const PatrolScreen()));
    await tester.pump();
    await expectLater(find.byType(MaterialApp), matchesGoldenFile('out/patrol_routes_phone.png'));
  });

  testWidgets('active tablet', (tester) async {
    h.actions.patrol = midway();
    await setSurface(tester, tabletSize);
    await tester.pumpWidget(wrap(h, const PatrolScreen()));
    await tester.pump();
    await expectLater(find.byType(MaterialApp), matchesGoldenFile('out/patrol_active_tablet.png'));
  });

  testWidgets('active phone', (tester) async {
    h.actions.patrol = midway();
    await setSurface(tester, phoneSize);
    await tester.pumpWidget(wrap(h, const PatrolScreen()));
    await tester.pump();
    await expectLater(find.byType(MaterialApp), matchesGoldenFile('out/patrol_active_phone.png'));
  });

  testWidgets('active english phone', (tester) async {
    h.actions.patrol = midway();
    await setSurface(tester, phoneSize);
    await tester.pumpWidget(wrap(h, const PatrolScreen(), locale: const Locale('en')));
    await tester.pump();
    await expectLater(find.byType(MaterialApp), matchesGoldenFile('out/patrol_active_phone_en.png'));
  });

  testWidgets('finish confirm', (tester) async {
    h.actions.patrol = midway();
    await setSurface(tester, tabletSize);
    await tester.pumpWidget(wrap(h, const PatrolScreen()));
    await tester.pump();
    await tester.tap(find.text('Selesaikan patroli').last);
    await tester.pumpAndSettle();
    await expectLater(find.byType(MaterialApp), matchesGoldenFile('out/patrol_finish_confirm.png'));
  });

  testWidgets('manual check-in sheet tablet', (tester) async {
    h.actions.patrol = midway();
    await setSurface(tester, tabletSize);
    await tester.pumpWidget(wrap(h, const PatrolScreen()));
    await tester.pump();
    await tester.tap(find.text('Tanpa QR (foto)'));
    await tester.pumpAndSettle();
    await expectLater(find.byType(MaterialApp), matchesGoldenFile('out/patrol_manual_tablet.png'));
  });

  testWidgets('manual check-in sheet phone', (tester) async {
    h.actions.patrol = midway();
    await setSurface(tester, phoneSize);
    await tester.pumpWidget(wrap(h, const PatrolScreen()));
    await tester.pump();
    await tester.tap(find.text('Tanpa QR (foto)'));
    await tester.pumpAndSettle();
    await expectLater(find.byType(MaterialApp), matchesGoldenFile('out/patrol_manual_phone.png'));
  });

  testWidgets('all scanned, then finished', (tester) async {
    h.actions.patrol = midway(all: true);
    await setSurface(tester, tabletSize);
    await tester.pumpWidget(wrap(h, const PatrolScreen()));
    await tester.pump();
    await expectLater(find.byType(MaterialApp), matchesGoldenFile('out/patrol_all_done_tablet.png'));
    await tester.tap(find.text('Selesaikan patroli').last);
    await tester.pump();
    await tester.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 300)));
    await tester.pump();
    expect(h.actions.patrol, isNull);
    await expectLater(find.byType(MaterialApp), matchesGoldenFile('out/patrol_completed_tablet.png'));
  });

  void mockScanner(WidgetTester tester, {required bool deny}) {
    final m = tester.binding.defaultBinaryMessenger;
    m.setMockMethodCallHandler(const MethodChannel('dev.steenbakker.mobile_scanner/scanner/event'), (_) async => null);
    m.setMockMethodCallHandler(const MethodChannel('dev.steenbakker.mobile_scanner/scanner/method'), (call) {
      if (!deny) return Completer<Object?>().future; // camera "starting" forever
      return Future.value(switch (call.method) { 'state' => 0, 'request' => false, _ => null });
    });
    addTearDown(() {
      m.setMockMethodCallHandler(const MethodChannel('dev.steenbakker.mobile_scanner/scanner/event'), null);
      m.setMockMethodCallHandler(const MethodChannel('dev.steenbakker.mobile_scanner/scanner/method'), null);
    });
  }

  for (final deny in [false, true]) {
    testWidgets('scanner (camera ${deny ? 'denied' : 'starting'})', (tester) async {
      h.actions.patrol = midway();
      mockScanner(tester, deny: deny);
      await setSurface(tester, tabletSize);
      await tester.pumpWidget(wrap(h, const ScannerScreen()));
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 100));
      await expectLater(find.byType(MaterialApp), matchesGoldenFile('out/patrol_scanner_${deny ? 'denied' : 'tablet'}.png'));
    });
  }

  testWidgets('scanner phone', (tester) async {
    h.actions.patrol = midway();
    mockScanner(tester, deny: false);
    await setSurface(tester, phoneSize);
    await tester.pumpWidget(wrap(h, const ScannerScreen()));
    await tester.pump();
    await expectLater(find.byType(MaterialApp), matchesGoldenFile('out/patrol_scanner_phone.png'));
  });
}
