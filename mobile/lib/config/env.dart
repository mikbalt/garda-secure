import 'package:flutter_dotenv/flutter_dotenv.dart';

class Env {
  static String get apiUrl =>
      dotenv.env['API_URL'] ?? 'http://localhost:8080';

  static String get keycloakUrl =>
      dotenv.env['KEYCLOAK_URL'] ?? 'http://localhost:8081';

  static String get keycloakRealm =>
      dotenv.env['KEYCLOAK_REALM'] ?? 'securepatrol';

  static String get keycloakClientId =>
      dotenv.env['KEYCLOAK_CLIENT_ID'] ?? 'mobile-app';
}
