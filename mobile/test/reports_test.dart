import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:securepatrol/features/field_actions.dart';
import 'package:securepatrol/features/incident/defect_screen.dart';
import 'package:securepatrol/features/incident/incident_screen.dart';
import 'package:securepatrol/features/incident/visitors_screen.dart';

import 'support/harness.dart';

/// Opens [screen] on top of a blank page so popping can be observed.
Widget _launcher(Harness h, Widget screen) => wrap(
      h,
      Builder(
        builder: (ctx) => Scaffold(
          body: Center(
            child: TextButton(
              onPressed: () => Navigator.of(ctx).push(MaterialPageRoute<void>(builder: (_) => screen)),
              child: const Text('open'),
            ),
          ),
        ),
      ),
    );

Finder _button(String label) => find.ancestor(of: find.text(label), matching: find.byWidgetPredicate((w) => w is ButtonStyleButton));

bool _enabled(WidgetTester tester, String label) => tester.widget<ButtonStyleButton>(_button(label).first).onPressed != null;

/// Taps, then alternates real waits (sqflite runs on real I/O) with pumps
/// (continuations live in the fake-async zone) until the work is done, and
/// lets the confirmation snackbar run out.
Future<void> _tapAndWait(WidgetTester tester, Finder f) async {
  await tester.tap(f);
  for (var i = 0; i < 40; i++) {
    await tester.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 60)));
    await tester.pump(const Duration(milliseconds: 100));
  }
  await tester.pumpAndSettle();
  await tester.pump(const Duration(seconds: 10));
  await tester.pumpAndSettle();
}

void main() {
  late Harness h;

  Future<void> open(WidgetTester tester, Widget screen) async {
    await setSurface(tester, tabletSize);
    h = (await tester.runAsync(Harness.create))!;
    h.seedSite();
    await tester.pumpWidget(_launcher(h, screen));
    await tester.tap(find.text('open'));
    await tester.pumpAndSettle();
  }

  Future<List<Map<String, dynamic>>> queued(WidgetTester tester, String kind) async {
    final ops = (await tester.runAsync(h.outbox.list))!;
    return ops.where((o) => o.kind == kind).map((o) => o.body).toList();
  }

  tearDown(() async => h.dispose());

  testWidgets('incident needs category and severity, then is queued', (tester) async {
    await open(tester, const IncidentScreen());
    expect(_enabled(tester, 'Kirim laporan'), isFalse);
    await tester.tap(find.text('Penyusupan'));
    await tester.pump();
    expect(_enabled(tester, 'Kirim laporan'), isFalse);
    await tester.tap(find.text('Tinggi'));
    await tester.pump();
    expect(_enabled(tester, 'Kirim laporan'), isTrue);
    await tester.enterText(find.byType(TextField).first, '  Dua orang memanjat pagar  ');
    FocusManager.instance.primaryFocus?.unfocus();
    await tester.pump();

    await _tapAndWait(tester, _button('Kirim laporan'));

    final bodies = await queued(tester, 'incident');
    expect(bodies, hasLength(1));
    expect(bodies.single['type'], 'trespassing');
    expect(bodies.single['severity'], 'high');
    expect(bodies.single['description'], 'Dua orang memanjat pagar');
    expect(bodies.single.containsKey('location'), isFalse);
    expect(find.byType(IncidentScreen), findsNothing, reason: 'closes after sending');
  });

  testWidgets('leaving a half-filled incident asks first', (tester) async {
    await open(tester, const IncidentScreen());
    await tester.tap(find.text('Medis'));
    await tester.pump();
    await tester.tap(find.byIcon(Icons.close_rounded));
    await tester.pumpAndSettle();
    expect(find.text('Buang laporan ini?'), findsOneWidget);
    await tester.tap(find.text('Lanjut mengisi'));
    await tester.pumpAndSettle();
    expect(find.byType(IncidentScreen), findsOneWidget);
    await tester.tap(find.byIcon(Icons.close_rounded));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Buang'));
    await tester.pumpAndSettle();
    expect(find.byType(IncidentScreen), findsNothing);
    expect(await queued(tester, 'incident'), isEmpty);
  });

  testWidgets('defect quick pick fills the type and is queued', (tester) async {
    await open(tester, const DefectScreen());
    expect(_enabled(tester, 'Kirim laporan'), isFalse);
    await tester.tap(find.text('Lampu'));
    await tester.pump();
    expect(_enabled(tester, 'Kirim laporan'), isTrue);

    await _tapAndWait(tester, _button('Kirim laporan'));

    final bodies = await queued(tester, 'defect');
    expect(bodies.single['defect_type'], 'Lampu');
    expect(find.byType(DefectScreen), findsNothing);
  });

  testWidgets('visitor in and out', (tester) async {
    await open(tester, const VisitorsScreen());
    expect(find.text('Belum ada tamu tercatat di tablet ini hari ini.'), findsOneWidget);
    await tester.tap(find.text('Tamu masuk').first);
    await tester.pumpAndSettle();
    expect(_enabled(tester, 'Catat tamu masuk'), isFalse);
    await tester.enterText(find.byType(TextField).at(0), 'Andi Wijaya');
    await tester.enterText(find.byType(TextField).at(3), 'b 9123 kxt');
    FocusManager.instance.primaryFocus?.unfocus();
    await tester.pump();

    await _tapAndWait(tester, _button('Catat tamu masuk'));

    final ins = await queued(tester, 'visitor');
    expect(ins.single['visitor_name'], 'Andi Wijaya');
    expect(ins.single['plate'], 'B 9123 KXT');
    expect(ins.single.containsKey('id_type'), isFalse, reason: 'preselected KTP is not sent without a number');
    expect(find.byType(VisitorInScreen), findsNothing);
    expect(find.text('Andi Wijaya'), findsOneWidget);
    expect(find.text('Masih di dalam'), findsOneWidget);

    await tester.tap(_button('Catat keluar').first);
    await tester.pumpAndSettle();
    await _tapAndWait(tester, _button('Catat keluar').last);

    final outs = (await tester.runAsync(h.outbox.list))!.where((o) => o.kind == 'visitor_exit').toList();
    expect(outs.single.path, '/visitors/${ins.single['id']}/exit');
    expect(h.actions.visitors.single.inside, isFalse);
    expect(find.text('Masih di dalam'), findsNothing);
    expect(h.actions.visitors, isA<List<LocalVisitor>>());
  });
}
