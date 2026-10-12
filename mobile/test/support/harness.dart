import 'dart:io';

import 'package:connectivity_plus/connectivity_plus.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:provider/provider.dart';
import 'package:securepatrol/config/env.dart';
import 'package:securepatrol/core/auth/auth_provider.dart';
import 'package:securepatrol/core/auth/keycloak_service.dart';
import 'package:securepatrol/core/database/local_db.dart';
import 'package:securepatrol/core/database/sync_service.dart';
import 'package:securepatrol/core/services/clock.dart';
import 'package:securepatrol/core/services/location_service.dart';
import 'package:securepatrol/core/services/photo_service.dart';
import 'package:securepatrol/core/storage/secure_store.dart';
import 'package:securepatrol/features/field_actions.dart';
import 'package:securepatrol/shared/models/models.dart';
import 'package:securepatrol/theme/app_theme.dart';
import 'package:sqflite_common_ffi/sqflite_ffi.dart';

import 'fake_api.dart';

/// Real app objects backed by an in-memory database, for widget tests and
/// screenshots. Nothing talks to a network: the API is a [FakeApi] (offline
/// by default) and the outbox is never started.
class Harness {
  Harness._(this.db, this.session, this.outbox, this.actions, this.api, this.store);

  final LocalDb db;
  final DeviceSession session;
  final Outbox outbox;
  final FieldActions actions;
  final FakeApi api;
  final MemorySecureStore store;

  static Future<Harness> create({FakeApi? api}) async {
    sqfliteFfiInit();
    AppClock.reset();
    final db = await LocalDb.open(factory: databaseFactoryFfi, path: inMemoryDatabasePath);
    final store = MemorySecureStore();
    final fake = api ?? FakeApi.offline();
    final session = DeviceSession(store: store, auth: KeycloakService(store), db: db, api: fake);
    session.config = const ServerConfig(serverUrl: 'https://patrol.test', realm: 'demo');
    final outbox = Outbox(
      db,
      fake,
      connectivity: const Stream<List<ConnectivityResult>>.empty(),
      onDeviceDenied: session.onDeviceDenied,
      onGuardTokenInvalid: session.onGuardTokenInvalid,
    );
    final actions = FieldActions(session: session, outbox: outbox, db: db, location: LocationService(), photos: PhotoService());
    return Harness._(db, session, outbox, actions, fake, store);
  }

  /// A typical active tablet at "Gudang Cikarang".
  void seedSite({bool unlocked = true}) {
    session.device = const Device(id: 'dev-1', name: 'Tablet Pos Utama', status: DeviceStatusKind.active, siteId: 'site-1');
    session.site = const Site(id: 'site-1', name: 'Gudang Cikarang', address: 'Kawasan Industri Jababeka Blok C-12');
    session.referenceSyncedAt = AppClock.now();
    session.roster = const [
      RosterGuard(id: 'g-1', name: 'Budi Santoso', employeeId: 'GS-0101', role: 'supervisor', pinHash: r'$2a$12$lFmYGucJNe.5H6xUWs6kiOPN2YrL89zSnPhr7fPwNpxA.G5zOIwc.'),
      RosterGuard(id: 'g-2', name: 'Agus Pratama', employeeId: 'GS-0102', pinHash: 'x'),
      RosterGuard(id: 'g-3', name: 'Dedi Kurniawan', employeeId: 'GS-0103', pinHash: 'x'),
      RosterGuard(id: 'g-4', name: 'Rina Wulandari', employeeId: 'GS-0104', pinHash: 'x'),
      RosterGuard(id: 'g-5', name: 'Joko Susilo', employeeId: 'GS-0105', pinHash: 'x'),
      RosterGuard(id: 'g-6', name: 'Hendra Saputra', employeeId: 'GS-0108'),
    ];
    session.routes = [
      const PatrolRoute(id: 'r-1', name: 'Patroli Keliling Gudang', checkpoints: [
        Checkpoint(id: 'c-1', name: 'Gerbang Utama', qrCode: 'SP-GATE-1', slaMinutes: 10, orderIndex: 0),
        Checkpoint(id: 'c-2', name: 'Dock Bongkar Muat', qrCode: 'SP-DOCK-2', slaMinutes: 15, orderIndex: 1),
        Checkpoint(id: 'c-3', name: 'Gudang Bahan Baku', qrCode: 'SP-WH-3', slaMinutes: 15, orderIndex: 2),
        Checkpoint(id: 'c-4', name: 'Ruang Panel Listrik', qrCode: 'SP-PANEL-4', slaMinutes: 10, orderIndex: 3),
        Checkpoint(id: 'c-5', name: 'Parkir Truk', qrCode: 'SP-PARK-5', slaMinutes: 15, orderIndex: 4),
      ]),
      const PatrolRoute(id: 'r-2', name: 'Patroli Malam Perimeter', checkpoints: [
        Checkpoint(id: 'c-6', name: 'Pos Jaga Timur', qrCode: 'SP-EAST-1', slaMinutes: 15, orderIndex: 0),
        Checkpoint(id: 'c-7', name: 'Pos Jaga Barat', qrCode: 'SP-WEST-2', slaMinutes: 20, orderIndex: 1),
      ]),
    ];
    if (unlocked) {
      session.guard = session.roster.first;
      session.phase = Phase.unlocked;
    } else {
      session.phase = Phase.locked;
    }
  }

  Future<void> dispose() async {
    await outbox.process(); // let a background send finish before closing
    outbox.dispose();
    await db.db.close();
  }
}

/// Wraps [child] in the providers, theme and localisation of the real app.
Widget wrap(Harness h, Widget child, {Locale locale = const Locale('id')}) {
  return MultiProvider(
    providers: [
      ChangeNotifierProvider<DeviceSession>.value(value: h.session),
      ChangeNotifierProvider<Outbox>.value(value: h.outbox),
      ChangeNotifierProvider<FieldActions>.value(value: h.actions),
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
}

/// Loads Inter and Material Icons so screenshots show real text and icons.
Future<void> loadAppFonts() async {
  final inter = FontLoader('Inter');
  for (final w in ['Regular', 'Medium', 'SemiBold', 'Bold']) {
    inter.addFont(Future.value(ByteData.sublistView(File('assets/fonts/Inter-$w.ttf').readAsBytesSync())));
  }
  await inter.load();
  final root = Platform.environment['FLUTTER_ROOT'];
  if (root != null) {
    final f = File('$root/bin/cache/artifacts/material_fonts/materialicons-regular.otf');
    if (f.existsSync()) {
      final icons = FontLoader('MaterialIcons')..addFont(Future.value(ByteData.sublistView(f.readAsBytesSync())));
      await icons.load();
    }
  }
}

/// Tablet landscape (10").
const tabletSize = Size(1280, 800);

/// Phone portrait.
const phoneSize = Size(400, 860);

Future<void> setSurface(WidgetTester tester, Size size) async {
  tester.view.physicalSize = size;
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
}
