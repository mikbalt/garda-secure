import 'package:geolocator/geolocator.dart';

/// A GPS fix attached to reports. Location is best-effort: a report is never
/// blocked because the tablet has no fix (indoor posts often have none).
class Fix {
  const Fix(this.lat, this.lng, this.accuracy);
  final double lat;
  final double lng;
  final double accuracy;
}

class LocationService {
  Fix? _last;
  DateTime? _lastAt;

  /// Current position within [timeout], or a fix younger than 2 minutes, or null.
  Future<Fix?> current({Duration timeout = const Duration(seconds: 8)}) async {
    try {
      if (!await Geolocator.isLocationServiceEnabled()) return _recent();
      var perm = await Geolocator.checkPermission();
      if (perm == LocationPermission.denied) perm = await Geolocator.requestPermission();
      if (perm == LocationPermission.denied || perm == LocationPermission.deniedForever) return _recent();
      final pos = await Geolocator.getCurrentPosition(
        locationSettings: const LocationSettings(accuracy: LocationAccuracy.high),
      ).timeout(timeout);
      _last = Fix(pos.latitude, pos.longitude, pos.accuracy);
      _lastAt = DateTime.now();
      return _last;
    } catch (_) {
      return _recent();
    }
  }

  /// The last fix taken by this app if younger than 2 minutes. Instant: no
  /// platform call, no permission prompt.
  Fix? lastKnown() => _recent();

  Fix? _recent() => _lastAt != null && DateTime.now().difference(_lastAt!) < const Duration(minutes: 2) ? _last : null;
}
