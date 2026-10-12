import 'package:flutter_secure_storage/flutter_secure_storage.dart';

/// Key-value store for secrets (tokens, server config). Abstracted so tests
/// can use [MemorySecureStore].
abstract class SecureStore {
  Future<String?> read(String key);
  Future<void> write(String key, String? value);
  Future<void> clear();
}

class DeviceSecureStore implements SecureStore {
  DeviceSecureStore() : _s = const FlutterSecureStorage(aOptions: AndroidOptions(encryptedSharedPreferences: true));

  final FlutterSecureStorage _s;

  @override
  Future<String?> read(String key) => _s.read(key: key);

  @override
  Future<void> write(String key, String? value) => value == null ? _s.delete(key: key) : _s.write(key: key, value: value);

  @override
  Future<void> clear() => _s.deleteAll();
}

class MemorySecureStore implements SecureStore {
  final Map<String, String> data = {};

  @override
  Future<String?> read(String key) async => data[key];

  @override
  Future<void> write(String key, String? value) async => value == null ? data.remove(key) : data[key] = value;

  @override
  Future<void> clear() async => data.clear();
}

/// Keys used in the secure store.
class SecureKeys {
  static const config = 'sp.config';
  static const refreshToken = 'sp.refresh';
  static const accessToken = 'sp.access';
  static const accessExpiry = 'sp.access.exp';
  static const language = 'sp.lang';
}
