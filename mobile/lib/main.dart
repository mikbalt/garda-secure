import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:provider/provider.dart';

import 'app.dart';
import 'core/auth/auth_provider.dart';
import 'core/auth/keycloak_service.dart';
import 'core/database/local_db.dart';
import 'core/database/sync_service.dart';
import 'core/services/location_service.dart';
import 'core/services/photo_service.dart';
import 'core/storage/secure_store.dart';
import 'features/field_actions.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  await SystemChrome.setPreferredOrientations(DeviceOrientation.values);

  final db = await LocalDb.open();
  final store = DeviceSecureStore();
  final auth = KeycloakService(store);
  final session = DeviceSession(store: store, auth: auth, db: db);
  final outbox = Outbox(db, session.api, onAuthExpired: () => unawaited(session.expire()));
  final actions = FieldActions(session: session, outbox: outbox, db: db, location: LocationService(), photos: PhotoService());

  runApp(MultiProvider(
    providers: [
      ChangeNotifierProvider<DeviceSession>.value(value: session),
      ChangeNotifierProvider<Outbox>.value(value: outbox),
      ChangeNotifierProvider<FieldActions>.value(value: actions),
    ],
    child: const SecurePatrolApp(),
  ));

  await session.boot();
  await outbox.start();
}
