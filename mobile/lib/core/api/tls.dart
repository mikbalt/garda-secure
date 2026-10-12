import 'dart:io';

import 'package:dio/dio.dart';
import 'package:dio/io.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';

/// TLS trust for every HTTP call of the app (MOB-8).
///
/// Dart's `HttpClient` (used by Dio for the API and for Keycloak) does **not**
/// use Android's network security config nor user-installed CAs: it trusts
/// the system roots only. An installation with a private CA (`make certs`)
/// therefore bundles its CA at build time as the asset
/// `assets/certs/securepatrol-ca.crt` (PEM); it is added to the system roots
/// here. Without that file only public CAs are trusted.
class TrustStore {
  TrustStore._();

  static const caAsset = 'assets/certs/securepatrol-ca.crt';

  static SecurityContext? _context;

  /// True when a bundled private CA was loaded.
  static bool get hasPrivateCa => _context != null;

  /// Plain `http://` server URLs are refused unless this is a debug build or
  /// the build sets `--dart-define=SP_ALLOW_HTTP=true` (lab use only).
  static bool allowHttp = kDebugMode || const bool.fromEnvironment('SP_ALLOW_HTTP');

  /// Loads the bundled CA, if the build has one.
  static Future<void> load({AssetBundle? bundle}) async {
    try {
      final data = await (bundle ?? rootBundle).load(caAsset);
      final bytes = data.buffer.asUint8List(data.offsetInBytes, data.lengthInBytes);
      if (bytes.isEmpty) return;
      _context = SecurityContext(withTrustedRoots: true)..setTrustedCertificatesBytes(bytes);
    } catch (_) {
      _context = null; // no bundled CA: system roots only
    }
  }

  /// A Dio whose connections trust the system roots plus the bundled CA.
  static Dio dio(BaseOptions options) {
    final d = Dio(options);
    d.httpClientAdapter = IOHttpClientAdapter(createHttpClient: () => HttpClient(context: _context));
    return d;
  }

  /// Rejects cleartext URLs (the device token and guard PINs travel there).
  static void checkUrl(String url) {
    if (url.startsWith('https://')) return;
    if (url.startsWith('http://') && allowHttp) return;
    throw const InsecureUrl();
  }

  static bool isAllowed(String url) {
    try {
      checkUrl(url);
      return true;
    } on InsecureUrl {
      return false;
    }
  }
}

class InsecureUrl implements Exception {
  const InsecureUrl();
  @override
  String toString() => 'https is required';
}
