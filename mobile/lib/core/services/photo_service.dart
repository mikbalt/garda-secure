import 'dart:io';

import 'package:image_picker/image_picker.dart';
import 'package:path/path.dart' as p;
import 'package:path_provider/path_provider.dart';
import 'package:uuid/uuid.dart';

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

class PhotoService {
  PhotoService({ImagePicker? picker}) : _picker = picker ?? ImagePicker();

  final ImagePicker _picker;
  static const _uuid = Uuid();

  /// Opens the camera; returns null when the guard cancels. Images are
  /// downscaled JPEGs well under the 10 MB upload limit.
  Future<LocalPhoto?> take({required String kind, required String siteId, bool front = false}) async {
    final shot = await _picker.pickImage(
      source: ImageSource.camera,
      preferredCameraDevice: front ? CameraDevice.front : CameraDevice.rear,
      maxWidth: 1600,
      maxHeight: 1600,
      imageQuality: 75,
    );
    if (shot == null) return null;
    final dir = Directory(p.join((await getApplicationDocumentsDirectory()).path, 'photos'));
    await dir.create(recursive: true);
    final id = _uuid.v4();
    final dest = p.join(dir.path, '$id.jpg');
    await File(shot.path).copy(dest);
    return LocalPhoto(id: id, path: dest, kind: kind, siteId: siteId);
  }

  /// Deletes a photo the guard discarded before submitting.
  Future<void> discard(LocalPhoto photo) async {
    try {
      await File(photo.path).delete();
    } catch (_) {}
  }
}
