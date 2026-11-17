// Screenshots of the report screens (not part of `flutter test test/`): run
//   flutter test screenshots/reports_shots_test.dart --update-goldens
// and look at screenshots/out/reports_*.png.
import 'dart:io';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:provider/provider.dart';
import 'package:securepatrol/core/auth/auth_provider.dart';
import 'package:securepatrol/core/database/sync_service.dart';
import 'package:securepatrol/core/services/location_service.dart';
import 'package:securepatrol/core/services/photo_service.dart';
import 'package:securepatrol/features/field_actions.dart';
import 'package:securepatrol/features/incident/defect_screen.dart';
import 'package:securepatrol/features/incident/incident_screen.dart';
import 'package:securepatrol/features/incident/visitors_screen.dart';
import 'package:securepatrol/theme/app_theme.dart';

import '../test/support/harness.dart';

/// Hands out pre-rendered images instead of opening the camera.
class FakePhotos extends PhotoService {
  FakePhotos(this.paths);

  final List<String> paths;
  int _n = 0;

  @override
  Future<LocalPhoto?> take({required String kind, required String siteId, bool front = false}) async => LocalPhoto(id: 'p${_n++}', path: paths[_n % paths.length], kind: kind, siteId: siteId);

  @override
  Future<void> discard(LocalPhoto photo) async {}
}

class NoGps extends LocationService {
  @override
  Future<Fix?> current({Duration timeout = const Duration(seconds: 8)}) async => null;
}

Future<String> _render(String path, Color a, Color b) async {
  final rec = ui.PictureRecorder();
  final c = Canvas(rec);
  const r = Rect.fromLTWH(0, 0, 320, 320);
  c.drawRect(r, Paint()..shader = ui.Gradient.linear(r.topLeft, r.bottomRight, [a, b]));
  c.drawCircle(const Offset(220, 110), 50, Paint()..color = Colors.white.withValues(alpha: 0.35));
  c.drawRect(const Rect.fromLTWH(30, 200, 200, 70), Paint()..color = Colors.black.withValues(alpha: 0.25));
  final img = await rec.endRecording().toImage(320, 320);
  final bytes = await img.toByteData(format: ui.ImageByteFormat.png);
  File(path).writeAsBytesSync(bytes!.buffer.asUint8List());
  return path;
}

late List<String> images;

/// Drops keyboard focus so the blinking caret does not keep pumpAndSettle busy.
Future<void> unfocus(WidgetTester tester) async {
  FocusManager.instance.primaryFocus?.unfocus();
  await tester.pump();
}

Widget app(Harness h, FieldActions actions, Widget child, {Locale locale = const Locale('id')}) => MultiProvider(
      providers: [
        ChangeNotifierProvider<DeviceSession>.value(value: h.session),
        ChangeNotifierProvider<Outbox>.value(value: h.outbox),
        ChangeNotifierProvider<FieldActions>.value(value: actions),
      ],
      child: MaterialApp(
        debugShowCheckedModeBanner: false,
        theme: AppTheme.light(),
        locale: locale,
        supportedLocales: const [Locale('id'), Locale('en')],
        localizationsDelegates: GlobalMaterialLocalizations.delegates,
        home: child,
      ),
    );

void main() {
  late Harness h;
  late FieldActions actions;

  setUpAll(() async {
    await loadAppFonts();
    final dir = Directory.systemTemp.createTempSync('sp_shots');
    images = [
      await _render('${dir.path}/a.png', const Color(0xFF334155), const Color(0xFF94A3B8)),
      await _render('${dir.path}/b.png', const Color(0xFF7C2D12), const Color(0xFFF59E0B)),
      await _render('${dir.path}/c.png', const Color(0xFF1E3A8A), const Color(0xFF60A5FA)),
    ];
  });

  setUp(() async {
    h = await Harness.create();
    h.seedSite();
    actions = FieldActions(session: h.session, outbox: h.outbox, db: h.db, location: NoGps(), photos: FakePhotos(images));
  });

  Future<void> precache(WidgetTester tester) async {
    final ctx = tester.element(find.byType(Scaffold).first);
    await tester.runAsync(() async {
      for (final p in images) {
        await precacheImage(FileImage(File(p)), ctx);
      }
    });
  }

  Future<void> shot(WidgetTester tester, String name) => expectLater(find.byType(MaterialApp), matchesGoldenFile('out/reports_$name.png'));

  Future<void> fillIncident(WidgetTester tester) async {
    await tester.tap(find.text('Penyusupan'));
    await tester.tap(find.text('Tinggi'));
    await tester.pump();
    await tester.enterText(find.byType(TextField).at(0), 'Dua orang memanjat pagar belakang dekat dock bongkar muat, lalu lari ke arah jalan raya. Satu memakai jaket hitam.');
    await tester.enterText(find.byType(TextField).at(1), 'Pagar belakang, dekat Dock Bongkar Muat');
    await unfocus(tester);
    await tester.ensureVisible(find.text('Ambil foto'));
    await tester.tap(find.text('Ambil foto'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Ambil foto'));
    await tester.pumpAndSettle();
    await tester.drag(find.byType(SingleChildScrollView), const Offset(0, 2000));
    await tester.pumpAndSettle();
  }

  testWidgets('incident empty tablet', (tester) async {
    await setSurface(tester, tabletSize);
    await tester.pumpWidget(app(h, actions, const IncidentScreen()));
    await tester.pumpAndSettle();
    await precache(tester);
    await shot(tester, 'incident_empty_tablet');
  });

  testWidgets('incident filled tablet (tall)', (tester) async {
    await setSurface(tester, const Size(1280, 1500));
    await tester.pumpWidget(app(h, actions, const IncidentScreen()));
    await tester.pumpAndSettle();
    await precache(tester);
    await fillIncident(tester);
    await shot(tester, 'incident_filled_tablet');
  });

  testWidgets('incident filled phone (tall)', (tester) async {
    await setSurface(tester, const Size(400, 1900));
    await tester.pumpWidget(app(h, actions, const IncidentScreen()));
    await tester.pumpAndSettle();
    await precache(tester);
    await tester.tap(find.text('Kritis'));
    await tester.tap(find.text('Bahaya kebakaran'));
    await tester.pump();
    await tester.ensureVisible(find.text('Ambil foto'));
    await tester.tap(find.text('Ambil foto'));
    await tester.pumpAndSettle();
    await shot(tester, 'incident_filled_phone');
  });

  testWidgets('incident empty phone', (tester) async {
    await setSurface(tester, phoneSize);
    await tester.pumpWidget(app(h, actions, const IncidentScreen()));
    await tester.pumpAndSettle();
    await precache(tester);
    await shot(tester, 'incident_empty_phone');
  });

  testWidgets('incident discard dialog', (tester) async {
    await setSurface(tester, tabletSize);
    await tester.pumpWidget(app(h, actions, const IncidentScreen()));
    await tester.pumpAndSettle();
    await precache(tester);
    await tester.tap(find.text('Pencurian'));
    await tester.pump();
    await tester.tap(find.byIcon(Icons.close_rounded));
    await tester.pumpAndSettle();
    await shot(tester, 'incident_discard_tablet');
  });

  testWidgets('incident english', (tester) async {
    await setSurface(tester, tabletSize);
    await tester.pumpWidget(app(h, actions, const IncidentScreen(), locale: const Locale('en')));
    await tester.pumpAndSettle();
    await precache(tester);
    await tester.tap(find.text('Medical'));
    await tester.tap(find.text('Critical'));
    await tester.pumpAndSettle();
    await shot(tester, 'incident_en_tablet');
  });

  testWidgets('defect tablet', (tester) async {
    await setSurface(tester, tabletSize);
    await tester.pumpWidget(app(h, actions, const DefectScreen()));
    await tester.pumpAndSettle();
    await precache(tester);
    await shot(tester, 'defect_empty_tablet');
    await tester.tap(find.text('Lampu'));
    await tester.enterText(find.byType(TextField).at(1), 'Parkir truk sisi utara');
    await tester.enterText(find.byType(TextField).at(2), 'Lampu sorot tiang 3 mati sejak semalam, area parkir gelap.');
    await unfocus(tester);
    await tester.ensureVisible(find.text('Ambil foto'));
    await tester.tap(find.text('Ambil foto'));
    await tester.pumpAndSettle();
    await setSurface(tester, const Size(1280, 1100));
    await tester.pumpAndSettle();
    await tester.drag(find.byType(SingleChildScrollView), const Offset(0, 2000));
    await tester.pumpAndSettle();
    await shot(tester, 'defect_filled_tablet');
  });

  testWidgets('defect phone', (tester) async {
    await setSurface(tester, phoneSize);
    await tester.pumpWidget(app(h, actions, const DefectScreen()));
    await tester.pumpAndSettle();
    await precache(tester);
    await tester.tap(find.text('CCTV'));
    await tester.pumpAndSettle();
    await shot(tester, 'defect_phone');
  });

  void seedVisitors() {
    final now = DateTime.now().toUtc();
    actions.visitors = [
      LocalVisitor(id: 'v1', name: 'Andi Wijaya', entryAt: now.subtract(const Duration(minutes: 25)), plate: 'B 9123 KXT', host: 'Bpk. Hartono (Gudang)', purpose: 'Antar barang — PT Sinar Logistik'),
      LocalVisitor(id: 'v2', name: 'Siti Rahmawati', entryAt: now.subtract(const Duration(hours: 2, minutes: 10)), host: 'HRD', purpose: 'Wawancara kerja'),
      LocalVisitor(id: 'v3', name: 'Teknisi AC — Yusuf', entryAt: now.subtract(const Duration(hours: 4, minutes: 5)), plate: 'B 4410 FAA', host: 'Bagian GA', purpose: 'Servis AC ruang panel'),
      LocalVisitor(
          id: 'v4',
          name: 'Rudi Hermawan',
          entryAt: now.subtract(const Duration(hours: 5)),
          exitAt: now.subtract(const Duration(hours: 3, minutes: 40)),
          plate: 'T 1777 BE',
          host: 'Ibu Lestari',
          purpose: 'Rapat vendor'),
      LocalVisitor(id: 'v5', name: 'Kurir JNE', entryAt: now.subtract(const Duration(hours: 6)), exitAt: now.subtract(const Duration(hours: 5, minutes: 50)), purpose: 'Ambil paket'),
    ];
  }

  testWidgets('visitors list tablet', (tester) async {
    await setSurface(tester, tabletSize);
    seedVisitors();
    await tester.pumpWidget(app(h, actions, const VisitorsScreen()));
    await tester.pumpAndSettle();
    await precache(tester);
    await shot(tester, 'visitors_tablet');
    await tester.tap(find.text('Catat keluar').first);
    await tester.pumpAndSettle();
    await shot(tester, 'visitors_exit_dialog_tablet');
  });

  testWidgets('visitors list phone', (tester) async {
    await setSurface(tester, phoneSize);
    seedVisitors();
    await tester.pumpWidget(app(h, actions, const VisitorsScreen()));
    await tester.pumpAndSettle();
    await precache(tester);
    await shot(tester, 'visitors_phone');
  });

  testWidgets('visitors empty tablet', (tester) async {
    await setSurface(tester, tabletSize);
    await tester.pumpWidget(app(h, actions, const VisitorsScreen()));
    await tester.pumpAndSettle();
    await precache(tester);
    await shot(tester, 'visitors_empty_tablet');
  });

  testWidgets('visitor form tablet', (tester) async {
    await setSurface(tester, tabletSize);
    await tester.pumpWidget(app(h, actions, const VisitorsScreen()));
    await tester.pumpAndSettle();
    await precache(tester);
    await tester.tap(find.text('Tamu masuk').first);
    await tester.pumpAndSettle();
    await shot(tester, 'visitor_form_empty_tablet');
    await tester.enterText(find.byType(TextField).at(0), 'Andi Wijaya');
    await tester.enterText(find.byType(TextField).at(1), '3216012345670001');
    await tester.enterText(find.byType(TextField).at(2), 'Bpk. Hartono (Gudang)');
    await tester.enterText(find.byType(TextField).at(3), 'B 9123 KXT');
    await tester.enterText(find.byType(TextField).at(4), 'Antar barang');
    await unfocus(tester);
    await tester.ensureVisible(find.text('Ambil foto'));
    await tester.tap(find.text('Ambil foto'));
    await tester.pumpAndSettle();
    await setSurface(tester, const Size(1280, 1150));
    await tester.pumpAndSettle();
    await tester.drag(find.byType(SingleChildScrollView), const Offset(0, 2000));
    await tester.pumpAndSettle();
    await shot(tester, 'visitor_form_filled_tablet');
  });

  testWidgets('visitor form phone', (tester) async {
    await setSurface(tester, phoneSize);
    await tester.pumpWidget(app(h, actions, const VisitorInScreen()));
    await tester.pumpAndSettle();
    await precache(tester);
    await shot(tester, 'visitor_form_phone');
  });
}
