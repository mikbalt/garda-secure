import 'package:sqflite/sqflite.dart';
import 'package:path/path.dart';

class LocalDb {
  static const _dbName = 'securepatrol.db';
  static const _dbVersion = 1;

  Database? _db;

  Database get db {
    if (_db == null) throw StateError('LocalDb not initialized. Call init() first.');
    return _db!;
  }

  Future<void> init() async {
    final dbPath = await getDatabasesPath();
    final path = join(dbPath, _dbName);

    _db = await openDatabase(
      path,
      version: _dbVersion,
      onCreate: _onCreate,
    );
  }

  Future<void> _onCreate(Database db, int version) async {
    await db.execute('''
      CREATE TABLE IF NOT EXISTS pending_clockins (
        id TEXT PRIMARY KEY,
        guard_id TEXT NOT NULL,
        shift_id TEXT NOT NULL,
        clock_in TEXT NOT NULL,
        clock_out TEXT,
        latitude REAL NOT NULL,
        longitude REAL NOT NULL,
        synced INTEGER NOT NULL DEFAULT 0,
        created_at TEXT NOT NULL DEFAULT (datetime('now'))
      )
    ''');

    await db.execute('''
      CREATE TABLE IF NOT EXISTS pending_scans (
        id TEXT PRIMARY KEY,
        guard_id TEXT NOT NULL,
        checkpoint_id TEXT NOT NULL,
        route_id TEXT NOT NULL,
        scanned_at TEXT NOT NULL,
        latitude REAL NOT NULL,
        longitude REAL NOT NULL,
        synced INTEGER NOT NULL DEFAULT 0,
        created_at TEXT NOT NULL DEFAULT (datetime('now'))
      )
    ''');

    await db.execute('''
      CREATE TABLE IF NOT EXISTS pending_incidents (
        id TEXT PRIMARY KEY,
        guard_id TEXT NOT NULL,
        site_id TEXT NOT NULL,
        type TEXT NOT NULL,
        description TEXT NOT NULL,
        latitude REAL NOT NULL,
        longitude REAL NOT NULL,
        photo_paths TEXT NOT NULL DEFAULT '[]',
        reported_at TEXT NOT NULL,
        synced INTEGER NOT NULL DEFAULT 0,
        created_at TEXT NOT NULL DEFAULT (datetime('now'))
      )
    ''');

    await db.execute('''
      CREATE TABLE IF NOT EXISTS cached_routes (
        id TEXT PRIMARY KEY,
        site_id TEXT NOT NULL,
        name TEXT NOT NULL,
        checkpoints_json TEXT NOT NULL DEFAULT '[]',
        cached_at TEXT NOT NULL DEFAULT (datetime('now'))
      )
    ''');

    await db.execute('''
      CREATE TABLE IF NOT EXISTS sos_queue (
        id TEXT PRIMARY KEY,
        guard_id TEXT NOT NULL,
        latitude REAL NOT NULL,
        longitude REAL NOT NULL,
        triggered_at TEXT NOT NULL,
        synced INTEGER NOT NULL DEFAULT 0
      )
    ''');
  }

  // --- pending_clockins ---

  Future<String> insertClockIn(Map<String, dynamic> data) async {
    await db.insert('pending_clockins', data,
        conflictAlgorithm: ConflictAlgorithm.replace);
    return data['id'] as String;
  }

  Future<List<Map<String, dynamic>>> getUnsyncedClockIns() async {
    return db.query('pending_clockins', where: 'synced = ?', whereArgs: [0]);
  }

  Future<void> markClockInSynced(String id) async {
    await db.update('pending_clockins', {'synced': 1},
        where: 'id = ?', whereArgs: [id]);
  }

  // --- pending_scans ---

  Future<String> insertScan(Map<String, dynamic> data) async {
    await db.insert('pending_scans', data,
        conflictAlgorithm: ConflictAlgorithm.replace);
    return data['id'] as String;
  }

  Future<List<Map<String, dynamic>>> getUnsyncedScans() async {
    return db.query('pending_scans', where: 'synced = ?', whereArgs: [0]);
  }

  Future<void> markScanSynced(String id) async {
    await db.update('pending_scans', {'synced': 1},
        where: 'id = ?', whereArgs: [id]);
  }

  // --- pending_incidents ---

  Future<String> insertIncident(Map<String, dynamic> data) async {
    await db.insert('pending_incidents', data,
        conflictAlgorithm: ConflictAlgorithm.replace);
    return data['id'] as String;
  }

  Future<List<Map<String, dynamic>>> getUnsyncedIncidents() async {
    return db.query('pending_incidents', where: 'synced = ?', whereArgs: [0]);
  }

  Future<void> markIncidentSynced(String id) async {
    await db.update('pending_incidents', {'synced': 1},
        where: 'id = ?', whereArgs: [id]);
  }

  // --- cached_routes ---

  Future<void> upsertRoute(Map<String, dynamic> data) async {
    await db.insert('cached_routes', data,
        conflictAlgorithm: ConflictAlgorithm.replace);
  }

  Future<List<Map<String, dynamic>>> getCachedRoutes() async {
    return db.query('cached_routes');
  }

  Future<void> clearRoutes() async {
    await db.delete('cached_routes');
  }

  // --- sos_queue ---

  Future<String> insertSosAlert(Map<String, dynamic> data) async {
    await db.insert('sos_queue', data,
        conflictAlgorithm: ConflictAlgorithm.replace);
    return data['id'] as String;
  }

  Future<List<Map<String, dynamic>>> getUnsyncedSosAlerts() async {
    return db.query('sos_queue', where: 'synced = ?', whereArgs: [0]);
  }

  Future<void> markSosSynced(String id) async {
    await db.update('sos_queue', {'synced': 1},
        where: 'id = ?', whereArgs: [id]);
  }

  Future<int> countPending() async {
    final clockIns = Sqflite.firstIntValue(await db.rawQuery(
            'SELECT COUNT(*) FROM pending_clockins WHERE synced = 0')) ??
        0;
    final scans = Sqflite.firstIntValue(await db.rawQuery(
            'SELECT COUNT(*) FROM pending_scans WHERE synced = 0')) ??
        0;
    final incidents = Sqflite.firstIntValue(await db.rawQuery(
            'SELECT COUNT(*) FROM pending_incidents WHERE synced = 0')) ??
        0;
    final sos = Sqflite.firstIntValue(
            await db.rawQuery('SELECT COUNT(*) FROM sos_queue WHERE synced = 0')) ??
        0;
    return clockIns + scans + incidents + sos;
  }
}
