import 'dart:convert';
import 'dart:io';
import 'package:connectivity_plus/connectivity_plus.dart';
import 'package:dio/dio.dart';
import 'local_db.dart';
import '../api/api_client.dart';

class SyncService {
  final LocalDb _localDb;
  final ApiClient _apiClient;

  SyncService({required LocalDb localDb, required ApiClient apiClient})
      : _localDb = localDb,
        _apiClient = apiClient;

  /// Run a full sync cycle: check connectivity, upload pending records,
  /// refresh cached routes.
  Future<SyncResult> sync() async {
    final connectivity = await Connectivity().checkConnectivity();
    if (connectivity == ConnectivityResult.none) {
      return SyncResult(synced: 0, failed: 0, online: false);
    }

    int synced = 0;
    int failed = 0;

    // Sync clock-ins
    final clockIns = await _localDb.getUnsyncedClockIns();
    for (final row in clockIns) {
      try {
        await _apiClient.dio.post('/api/v1/attendance/clock-in', data: row);
        await _localDb.markClockInSynced(row['id'] as String);
        synced++;
      } catch (_) {
        failed++;
      }
    }

    // Sync checkpoint scans
    final scans = await _localDb.getUnsyncedScans();
    for (final row in scans) {
      try {
        await _apiClient.dio.post('/api/v1/patrol/scans', data: row);
        await _localDb.markScanSynced(row['id'] as String);
        synced++;
      } catch (_) {
        failed++;
      }
    }

    // Sync incidents (with photos)
    final incidents = await _localDb.getUnsyncedIncidents();
    for (final row in incidents) {
      try {
        // Upload photos first if any
        final photoPaths =
            jsonDecode(row['photo_paths'] as String) as List<dynamic>;
        final uploadedUrls = <String>[];
        for (final path in photoPaths) {
          try {
            final formData = FormData.fromMap({
              'file': await MultipartFile.fromFile(path as String),
            });
            final uploadResponse =
                await _apiClient.dio.post('/api/v1/media/upload', data: formData);
            uploadedUrls
                .add(uploadResponse.data['url'] as String);
          } catch (_) {
            // Skip failed photo uploads; incident still syncs without them
          }
        }

        final incidentData = Map<String, dynamic>.from(row);
        incidentData['photo_urls'] = uploadedUrls;
        incidentData.remove('photo_paths');

        await _apiClient.dio.post('/api/v1/incidents', data: incidentData);
        await _localDb.markIncidentSynced(row['id'] as String);
        synced++;
      } catch (_) {
        failed++;
      }
    }

    // Sync SOS queue
    final sosAlerts = await _localDb.getUnsyncedSosAlerts();
    for (final row in sosAlerts) {
      try {
        await _apiClient.dio.post('/api/v1/sos', data: row);
        await _localDb.markSosSynced(row['id'] as String);
        synced++;
      } catch (_) {
        failed++;
      }
    }

    // Refresh cached routes
    try {
      final routesResponse = await _apiClient.dio.get('/api/v1/routes');
      final routes = routesResponse.data as List<dynamic>;
      await _localDb.clearRoutes();
      for (final route in routes) {
        final routeMap = route as Map<String, dynamic>;
        await _localDb.upsertRoute({
          'id': routeMap['id'],
          'site_id': routeMap['site_id'],
          'name': routeMap['name'],
          'checkpoints_json': jsonEncode(routeMap['checkpoints'] ?? []),
          'cached_at': DateTime.now().toIso8601String(),
        });
      }
    } catch (_) {
      // Route refresh failure is non-fatal; cached routes remain usable
    }

    return SyncResult(synced: synced, failed: failed, online: true);
  }
}

class SyncResult {
  final int synced;
  final int failed;
  final bool online;

  SyncResult({required this.synced, required this.failed, required this.online});
}
