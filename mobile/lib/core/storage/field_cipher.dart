import 'dart:convert';
import 'dart:math';
import 'dart:typed_data';

import 'package:cryptography/cryptography.dart';
import 'package:cryptography/dart.dart';

import 'secure_store.dart';

/// Thrown when a sealed value cannot be opened (wrong key or tampered data).
class CipherError implements Exception {
  const CipherError(this.message);
  final String message;
  @override
  String toString() => 'CipherError: $message';
}

/// Encrypts sensitive values stored in the local SQLite database (MOB-6):
/// every `kv` cache value (roster with PIN hashes, routes, guard state,
/// visitors…) and the outbox request bodies, labels and guard tokens.
///
/// AES-256-GCM (pure Dart, synchronous so row mapping stays simple) with a
/// random 96-bit nonce per value. The key is generated once and kept in the
/// platform secure storage (Android Keystore-backed), never in the database.
///
/// Sealed format: `e1:` + base64(nonce ‖ ciphertext ‖ mac). Values without the
/// prefix are legacy plaintext and are returned as-is so an existing database
/// can be migrated in place ([LocalDb] re-seals them on open).
class FieldCipher {
  FieldCipher(List<int> key) : _key = SecretKeyData(Uint8List.fromList(key)) {
    if (key.length != 32) throw ArgumentError('key must be 32 bytes');
  }

  final SecretKeyData _key;
  static final _aes = DartAesGcm.with256bits();
  static const prefix = 'e1:';
  static const _nonceLength = 12;
  static const _macLength = 16;

  /// Loads the database key from [store], creating it on first use.
  static Future<FieldCipher> load(SecureStore store) async {
    final existing = await store.read(SecureKeys.dbKey);
    if (existing != null) {
      try {
        final bytes = base64Decode(existing);
        if (bytes.length == 32) return FieldCipher(bytes);
      } catch (_) {}
    }
    final rnd = Random.secure();
    final key = List<int>.generate(32, (_) => rnd.nextInt(256));
    await store.write(SecureKeys.dbKey, base64Encode(key));
    return FieldCipher(key);
  }

  static bool isSealed(String? v) => v != null && v.startsWith(prefix);

  String seal(String plain) {
    final rnd = Random.secure();
    final nonce = List<int>.generate(_nonceLength, (_) => rnd.nextInt(256));
    final box = _aes.encryptSync(utf8.encode(plain), secretKeyData: _key, nonce: nonce);
    final out = BytesBuilder(copy: false)
      ..add(box.nonce)
      ..add(box.cipherText)
      ..add(box.mac.bytes);
    return '$prefix${base64Encode(out.toBytes())}';
  }

  /// Decrypts a sealed value; legacy plaintext passes through unchanged.
  String open(String stored) {
    if (!isSealed(stored)) return stored;
    final List<int> raw;
    try {
      raw = base64Decode(stored.substring(prefix.length));
    } on FormatException {
      throw const CipherError('not base64');
    }
    if (raw.length < _nonceLength + _macLength) throw const CipherError('too short');
    final box = SecretBox(
      raw.sublist(_nonceLength, raw.length - _macLength),
      nonce: raw.sublist(0, _nonceLength),
      mac: Mac(raw.sublist(raw.length - _macLength)),
    );
    try {
      return utf8.decode(_aes.decryptSync(box, secretKeyData: _key));
    } on SecretBoxAuthenticationError {
      throw const CipherError('authentication failed');
    }
  }

  String? sealNullable(String? plain) => plain == null ? null : seal(plain);
  String? openNullable(String? stored) => stored == null ? null : open(stored);
}
