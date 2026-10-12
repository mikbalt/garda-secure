/// Server configuration of the tablet.
///
/// Defaults come from `--dart-define` (e.g. `--dart-define=SP_SERVER=https://10.0.0.5`)
/// so a build can be preset for one installation; the operator can still
/// change them on the first-run setup screen. The chosen values are stored in
/// secure storage (see [DeviceSession]).
class ServerConfig {
  const ServerConfig({
    required this.serverUrl,
    required this.realm,
    this.keycloakUrl,
    this.clientId = defaultClientId,
  });

  /// Base URL of the installation, e.g. `https://patrol.example.co.id`.
  final String serverUrl;

  /// Keycloak realm of the client company (one realm per client, D-005).
  final String realm;

  /// Keycloak base URL; defaults to `<serverUrl>/auth`.
  final String? keycloakUrl;

  /// Public Keycloak client for tablets (password grant + offline token).
  final String clientId;

  static const defaultClientId = 'securepatrol-tablet';

  /// Build-time defaults.
  static const presetServer = String.fromEnvironment('SP_SERVER');
  static const presetRealm = String.fromEnvironment('SP_REALM');
  static const presetKeycloak = String.fromEnvironment('SP_KEYCLOAK');

  String get _base => serverUrl.replaceAll(RegExp(r'/+$'), '');

  /// REST API base (already includes `/api/v1`).
  String get apiBase => '$_base/api/v1';

  String get keycloakBase => (keycloakUrl?.isNotEmpty ?? false) ? keycloakUrl!.replaceAll(RegExp(r'/+$'), '') : '$_base/auth';

  String get tokenEndpoint => '$keycloakBase/realms/$realm/protocol/openid-connect/token';

  Map<String, String> toJson() => {
        'server_url': serverUrl,
        'realm': realm,
        if (keycloakUrl != null) 'keycloak_url': keycloakUrl!,
        'client_id': clientId,
      };

  static ServerConfig? fromJson(Map<String, dynamic>? j) {
    if (j == null || j['server_url'] == null || j['realm'] == null) return null;
    return ServerConfig(
      serverUrl: j['server_url'] as String,
      realm: j['realm'] as String,
      keycloakUrl: j['keycloak_url'] as String?,
      clientId: (j['client_id'] as String?) ?? defaultClientId,
    );
  }

  /// Normalises what an operator types: adds https:// when missing.
  static String normalizeUrl(String input) {
    var s = input.trim();
    if (s.isEmpty) return s;
    if (!s.startsWith('http://') && !s.startsWith('https://')) s = 'https://$s';
    return s.replaceAll(RegExp(r'/+$'), '');
  }
}
