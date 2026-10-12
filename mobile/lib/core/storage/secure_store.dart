import 'package:flutter_secure_storage/flutter_secure_storage.dart';

/// Key-value store for secrets (tokens, server config). Abstracted so tests
/// can use [MemorySecureStore].
abstract class SecureStore {
  Future<String?> read(String key);
  Future<void> write(String key, String? value);
  Future<void> clear();

  /// Deletes every key starting with [prefix].
  Future<void> deletePrefix(String prefix);
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

  @override
  Future<void> deletePrefix(String prefix) async {
    final all = await _s.readAll();
    for (final k in all.keys.where((k) => k.startsWith(prefix))) {
      await _s.delete(key: k);
    }
  }
}

class MemorySecureStore implements SecureStore {
  final Map<String, String> data = {};

  @override
  Future<String?> read(String key) async => data[key];

  @override
  Future<void> write(String key, String? value) async => value == null ? data.remove(key) : data[key] = value;

  @override
  Future<void> clear() async => data.clear();

  @override
  Future<void> deletePrefix(String prefix) async => data.removeWhere((k, _) => k.startsWith(prefix));
}

/// Keys used in the secure store.
class SecureKeys {
  static const config = 'sp.config';
  static const refreshToken = 'sp.refresh';
  static const accessToken = 'sp.access';
  static const accessExpiry = 'sp.access.exp';
  static const language = 'sp.lang';

  /// AES key of the local database fields (MOB-6, [FieldCipher]).
  static const dbKey = 'sp.dbkey';

  /// Server-issued guard session token of one guard (D-035).
  static String guardToken(String guardId) => 'sp.gt.$guardId';
}
