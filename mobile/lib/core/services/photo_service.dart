import 'dart:io';

import 'package:image_picker/image_picker.dart';
import 'package:path/path.dart' as p;
import 'package:path_provider/path_provider.dart';
import 'package:uuid/uuid.dart';

import 'jpeg_meta.dart';

/// A photo taken on the tablet, kept in app storage until it is uploaded.
class LocalPhoto {
  const LocalPhoto({required this.id, required this.path, required this.kind, required this.siteId});

  final String id;
  final String path;

  /// selfie | checkpoint | incident | defect | visitor
  final String kind;
  final String siteId;

  /// Storage key the server will use (D-012); records may reference it before
  /// the upload happens.
  String get objectKey => 'sites/$siteId/$kind/$id.jpg';
}

/// Tracks screens of other apps opened on purpose (the camera), so going to
/// the background for them does not lock the tablet (MOB-11).
class ExternalActivity {
  ExternalActivity._();

  static int _depth = 0;

  static bool get active => _depth > 0;

  static Future<T> run<T>(Future<T> Function() f) async {
    _depth++;
    try {
      return await f();
    } finally {
      _depth--;
    }
  }
}

class PhotoService {
  PhotoService({ImagePicker? picker}) : _picker = picker ?? ImagePicker();

  final ImagePicker _picker;
  static const _uuid = Uuid();

  static Future<Directory> photosDir() async => Directory(p.join((await getApplicationDocumentsDirectory()).path, 'photos'));

  /// Opens the camera; returns null when the guard cancels. Images are
  /// downscaled JPEGs well under the 10 MB upload limit, without EXIF/GPS
  /// metadata; the camera's temporary file is deleted.
  Future<LocalPhoto?> take({required String kind, required String siteId, bool front = false}) async {
    final shot = await ExternalActivity.run(() => _picker.pickImage(
          source: ImageSource.camera,
          preferredCameraDevice: front ? CameraDevice.front : CameraDevice.rear,
          maxWidth: 1600,
          maxHeight: 1600,
          imageQuality: 75,
        ));
    if (shot == null) return null;
    final dir = await photosDir();
    await dir.create(recursive: true);
    final id = _uuid.v4();
    final dest = p.join(dir.path, '$id.jpg');
    final tmp = File(shot.path);
    try {
      await File(dest).writeAsBytes(JpegMeta.strip(await tmp.readAsBytes()), flush: true);
    } finally {
      try {
        await tmp.delete(); // image_picker's cache copy
      } catch (_) {}
    }
    return LocalPhoto(id: id, path: dest, kind: kind, siteId: siteId);
  }

  /// Deletes a photo the guard discarded before submitting.
  Future<void> discard(LocalPhoto photo) async {
    try {
      await File(photo.path).delete();
    } catch (_) {}
  }

  /// Deletes photo files no queued item refers to (left over by a crash or a
  /// form that was closed). Run at start-up, before any form is open.
  static Future<int> cleanOrphans(Set<String> referenced, {Directory? dir}) async {
    final d = dir ?? await photosDir();
    if (!d.existsSync()) return 0;
    final keep = referenced.map(p.normalize).toSet();
    var n = 0;
    for (final f in d.listSync().whereType<File>()) {
      if (keep.contains(p.normalize(f.path))) continue;
      try {
        f.deleteSync();
        n++;
      } catch (_) {}
    }
    return n;
  }
}
