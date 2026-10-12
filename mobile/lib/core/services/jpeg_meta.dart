import 'dart:typed_data';

/// Removes metadata from a JPEG without re-encoding it (MOB-11).
///
/// `image_picker` re-encodes resized photos but copies the original EXIF
/// (camera model, time, **GPS position**) into the result. Decoding and
/// re-encoding in Dart would cost seconds on a tablet, so this walks the JPEG
/// segments instead (linear copy, a few ms):
/// - dropped: APP1 (EXIF, XMP), APP2 other than an ICC profile (e.g. MPF
///   thumbnails), APP3–APP13 (incl. IPTC), APP15, comments;
/// - kept: APP0 (JFIF), the ICC profile, APP14 (Adobe colour transform),
///   all image data;
/// - the EXIF orientation is re-added as a minimal EXIF block so photos are
///   not shown sideways (the server keeps it as well).
///
/// Anything that does not parse as a JPEG is returned unchanged (the server
/// strips metadata too).
class JpegMeta {
  JpegMeta._();

  static Uint8List strip(Uint8List src) {
    try {
      return _strip(src) ?? src;
    } catch (_) {
      return src;
    }
  }

  /// EXIF orientation (1–8) of a JPEG, or null.
  static int? orientationOf(Uint8List src) {
    try {
      for (final s in _segments(src) ?? const <_Segment>[]) {
        if (s.marker == 0xE1) {
          final o = _orientation(src, s.start + 4, s.end);
          if (o != null) return o;
        }
      }
    } catch (_) {}
    return null;
  }

  static List<_Segment>? _segments(Uint8List b) {
    if (b.length < 4 || b[0] != 0xFF || b[1] != 0xD8) return null;
    final out = <_Segment>[];
    var i = 2;
    while (i < b.length) {
      if (b[i] != 0xFF) return null;
      // Fill bytes.
      while (i < b.length && b[i] == 0xFF) {
        i++;
      }
      if (i >= b.length) return null;
      final marker = b[i];
      final start = i - 1;
      i++;
      if (marker == 0xD9 || (marker >= 0xD0 && marker <= 0xD7) || marker == 0x01) {
        out.add(_Segment(marker, start, i));
        if (marker == 0xD9) break;
        continue;
      }
      if (i + 2 > b.length) return null;
      final len = (b[i] << 8) | b[i + 1];
      if (len < 2 || i + len > b.length) return null;
      if (marker == 0xDA) {
        // Start of scan: the rest is entropy-coded data up to EOI.
        out.add(_Segment(marker, start, b.length));
        break;
      }
      out.add(_Segment(marker, start, i + len));
      i += len;
    }
    return out;
  }

  static bool _startsWith(Uint8List b, int at, List<int> prefix) {
    if (at + prefix.length > b.length) return false;
    for (var k = 0; k < prefix.length; k++) {
      if (b[at + k] != prefix[k]) return false;
    }
    return true;
  }

  static const _exif = [0x45, 0x78, 0x69, 0x66, 0x00, 0x00]; // "Exif\0\0"
  static const _icc = [0x49, 0x43, 0x43, 0x5F, 0x50, 0x52, 0x4F, 0x46, 0x49, 0x4C, 0x45, 0x00]; // "ICC_PROFILE\0"

  /// Orientation from an APP1 payload [from, to) (after the length bytes).
  static int? _orientation(Uint8List b, int from, int to) {
    if (!_startsWith(b, from, _exif)) return null;
    final t = from + 6; // TIFF header
    if (t + 8 > to) return null;
    final little = b[t] == 0x49 && b[t + 1] == 0x49;
    final big = b[t] == 0x4D && b[t + 1] == 0x4D;
    if (!little && !big) return null;
    int u16(int at) => little ? b[at] | (b[at + 1] << 8) : (b[at] << 8) | b[at + 1];
    int u32(int at) => little
        ? b[at] | (b[at + 1] << 8) | (b[at + 2] << 16) | (b[at + 3] << 24)
        : (b[at] << 24) | (b[at + 1] << 16) | (b[at + 2] << 8) | b[at + 3];
    final ifd = t + u32(t + 4);
    if (ifd + 2 > to) return null;
    final n = u16(ifd);
    for (var k = 0; k < n; k++) {
      final e = ifd + 2 + k * 12;
      if (e + 12 > to) return null;
      if (u16(e) == 0x0112 && u16(e + 2) == 3) {
        final v = u16(e + 8);
        return v >= 1 && v <= 8 ? v : null;
      }
    }
    return null;
  }

  static Uint8List _minimalExif(int orientation) => Uint8List.fromList([
        0xFF, 0xE1, 0x00, 0x22, // APP1, length 34
        ..._exif,
        0x4D, 0x4D, 0x00, 0x2A, 0x00, 0x00, 0x00, 0x08, // big-endian TIFF, IFD0 at 8
        0x00, 0x01, // one entry
        0x01, 0x12, 0x00, 0x03, 0x00, 0x00, 0x00, 0x01, 0x00, orientation, 0x00, 0x00, // Orientation SHORT
        0x00, 0x00, 0x00, 0x00, // no next IFD
      ]);

  static Uint8List? _strip(Uint8List b) {
    final segs = _segments(b);
    if (segs == null) return null;
    int? orientation;
    for (final s in segs) {
      if (s.marker == 0xE1) orientation ??= _orientation(b, s.start + 4, s.end);
    }
    bool keep(_Segment s) {
      final m = s.marker;
      if (m == 0xFE) return false; // comment
      if (m == 0xE1) return false; // EXIF / XMP
      if (m == 0xE2) return _startsWith(b, s.start + 4, _icc); // ICC only
      if (m >= 0xE3 && m <= 0xED) return false;
      if (m == 0xEF) return false;
      return true; // APP0, APP14, tables, frame, scan, …
    }

    final out = BytesBuilder(copy: false)..add(const [0xFF, 0xD8]);
    var inserted = orientation == null || orientation == 1;
    for (final s in segs) {
      if (!inserted && s.marker != 0xE0) {
        out.add(_minimalExif(orientation!));
        inserted = true;
      }
      if (keep(s)) out.add(Uint8List.sublistView(b, s.start, s.end));
    }
    return out.toBytes();
  }
}

class _Segment {
  const _Segment(this.marker, this.start, this.end);

  final int marker;

  /// Offset of the 0xFF of the marker.
  final int start;

  /// End (exclusive) of the segment.
  final int end;
}
