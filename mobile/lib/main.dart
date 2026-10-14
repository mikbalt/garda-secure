import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:provider/provider.dart';

import 'app.dart';
import 'core/api/tls.dart';
import 'core/auth/auth_provider.dart';
import 'core/auth/keycloak_service.dart';
import 'core/database/local_db.dart';
import 'core/database/sync_service.dart';
import 'core/services/health.dart';
import 'core/services/location_service.dart';
import 'core/services/photo_service.dart';
import 'core/services/push_service.dart';
import 'core/storage/field_cipher.dart';
import 'core/storage/secure_store.dart';
import 'features/field_actions.dart';

/// Errors before the database is open (or if it never opens).
ErrorLog? _errors;

void main() {
  // Uncaught async errors land here (MOB-7): logged locally, shipped in the
  // health beacon; no external crash service.
  runZonedGuarded(_main, (error, stack) {
    debugPrint('uncaught: $error');
    _errors?.record(error, stack, source: 'zone');
  });
}

Future<void> _main() async {
  WidgetsFlutterBinding.ensureInitialized();
  FlutterError.onError = (details) {
    FlutterError.presentError(details);
    _errors?.record(details.exception, details.stack, source: 'flutter');
  };
  PlatformDispatcher.instance.onError = (error, stack) {
    _errors?.record(error, stack, source: 'platform');
    return true;
  };
  await SystemChrome.setPreferredOrientations(DeviceOrientation.values);

  await TrustStore.load();
  await AppInfo.load();
  final store = DeviceSecureStore();
  final db = await LocalDb.open(cipher: await FieldCipher.load(store));
  final errors = _errors = ErrorLog(db);
  await errors.load();
  if (db.keyReset) errors.record(StateError('database key lost; local data dropped'), null, source: 'db');

  final auth = KeycloakService(store);
  final session = DeviceSession(store: store, auth: auth, db: db);
  final outbox = Outbox(
    db,
    session.api,
    onAuthExpired: () => unawaited(session.expire()),
    onDeviceDenied: session.onDeviceDenied,
    onGuardTokenInvalid: session.onGuardTokenInvalid,
  );
  session.onDeviceActive = outbox.resume;
  // Firebase Cloud Messaging when the build has the SP_FCM_* values;
  // otherwise disabled and polling works as before.
  final push = PushService(session: session)..attach();
  push.onOpen = (payload) => openPushTarget(session, payload);
  push.onError = (e, st) => errors.record(e, st, source: 'push');
  final actions = FieldActions(session: session, outbox: outbox, db: db, location: LocationService(), photos: PhotoService());
  final beacon = HealthBeacon(
    api: session.api,
    errors: errors,
    active: () => session.phase == Phase.locked || session.phase == Phase.unlocked,
    snapshot: () => HealthSnapshot(
      outboxPending: outbox.pending,
      outboxFailed: outbox.failed,
      outboxPaused: outbox.pauseCode,
      sosPending: outbox.sosPending,
      lastSync: outbox.lastSuccess,
      rosterSyncedAt: session.referenceSyncedAt,
      pinFailures: session.pinAttempts.failureCounts,
      dbKeyReset: db.keyReset,
      push: push.beaconStatus,
    ),
  );

  runApp(MultiProvider(
    providers: [
      ChangeNotifierProvider<DeviceSession>.value(value: session),
      ChangeNotifierProvider<Outbox>.value(value: outbox),
      ChangeNotifierProvider<FieldActions>.value(value: actions),
      ChangeNotifierProvider<PushService>.value(value: push),
    ],
    child: const SecurePatrolApp(),
  ));

  await session.boot();
  await outbox.start();
  beacon.start();
  unawaited(push.start());
  // Photos of reports that were never queued (app killed mid-form) or whose
  // item is gone: nothing refers to them any more (MOB-11).
  unawaited(outbox.photoPaths().then(PhotoService.cleanOrphans).catchError((Object _) => 0));
}
