import 'package:flutter/material.dart';
import 'package:flutter_dotenv/flutter_dotenv.dart';
import 'package:provider/provider.dart';
import 'app.dart';
import 'core/auth/auth_provider.dart';
import 'core/auth/keycloak_service.dart';
import 'core/api/api_client.dart';
import 'core/database/local_db.dart';
import 'core/database/sync_service.dart';
import 'config/env.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();

  // Load .env
  await dotenv.load(fileName: '.env');

  // Init local SQLite database
  await LocalDb.instance.init();

  final env = AppEnv();
  final keycloakService = KeycloakService(
    keycloakUrl: env.keycloakUrl,
    realm: env.keycloakRealm,
    clientId: env.keycloakClientId,
  );
  final apiClient = ApiClient(baseUrl: env.apiUrl);
  final syncService = SyncService(apiClient: apiClient, db: LocalDb.instance);

  runApp(
    MultiProvider(
      providers: [
        Provider<ApiClient>.value(value: apiClient),
        Provider<SyncService>.value(value: syncService),
        ChangeNotifierProvider(
          create: (_) => AuthProvider(keycloakService: keycloakService),
        ),
      ],
      child: const SecurePatrolApp(),
    ),
  );
}
