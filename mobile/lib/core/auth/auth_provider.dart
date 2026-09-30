import 'package:flutter/foundation.dart';
import 'keycloak_service.dart';
import '../../core/api/api_client.dart';

class AuthProvider extends ChangeNotifier {
  final KeycloakService _keycloakService;
  final ApiClient _apiClient;

  bool _isAuthenticated = false;
  String? _guardId;
  String? _guardName;
  List<String> _roles = [];
  bool _isLoading = false;
  String? _error;

  AuthProvider({
    required KeycloakService keycloakService,
    required ApiClient apiClient,
  })  : _keycloakService = keycloakService,
        _apiClient = apiClient;

  bool get isAuthenticated => _isAuthenticated;
  String? get guardId => _guardId;
  String? get guardName => _guardName;
  List<String> get roles => List.unmodifiable(_roles);
  bool get isLoading => _isLoading;
  String? get error => _error;

  /// Authenticate guard using their ID (as username) and PIN (as password).
  Future<void> loginWithPin(String guardId, String pin) async {
    _isLoading = true;
    _error = null;
    notifyListeners();

    try {
      await _keycloakService.login(guardId, pin);

      // Fetch guard profile from API
      final response = await _apiClient.dio.get('/api/v1/guards/me');
      final data = response.data as Map<String, dynamic>;

      _guardId = data['id'] as String?;
      _guardName = data['name'] as String?;
      _roles = List<String>.from(data['roles'] as List? ?? []);
      _isAuthenticated = true;
      _error = null;
    } catch (e) {
      _isAuthenticated = false;
      _guardId = null;
      _guardName = null;
      _roles = [];
      _error = _parseError(e);
    } finally {
      _isLoading = false;
      notifyListeners();
    }
  }

  Future<void> logout() async {
    await _keycloakService.logout();
    _isAuthenticated = false;
    _guardId = null;
    _guardName = null;
    _roles = [];
    _error = null;
    notifyListeners();
  }

  /// Check stored token on app start and restore session if valid.
  Future<void> checkAuthStatus() async {
    final authenticated = await _keycloakService.isAuthenticated();
    if (authenticated) {
      try {
        final response = await _apiClient.dio.get('/api/v1/guards/me');
        final data = response.data as Map<String, dynamic>;
        _guardId = data['id'] as String?;
        _guardName = data['name'] as String?;
        _roles = List<String>.from(data['roles'] as List? ?? []);
        _isAuthenticated = true;
      } catch (_) {
        await logout();
      }
    } else {
      _isAuthenticated = false;
    }
    notifyListeners();
  }

  String _parseError(Object e) {
    if (e is Exception) {
      final msg = e.toString();
      if (msg.contains('401') || msg.contains('Unauthorized')) {
        return 'Invalid guard ID or PIN';
      }
      if (msg.contains('SocketException') || msg.contains('Connection')) {
        return 'No network connection';
      }
      return 'Login failed. Please try again.';
    }
    return 'An unexpected error occurred';
  }
}
