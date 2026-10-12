import 'dart:convert';

import 'package:path/path.dart' as p;
import 'package:sqflite/sqflite.dart';

/// On-device SQLite storage:
/// - `outbox`: every field action waiting to be sent (see [Outbox]).
/// - `kv`: JSON caches (device, site, roster, routes, per-guard state, …) so
///   the tablet keeps working offline.
class LocalDb {
  LocalDb._(this.db);

  final Database db;

  static const _version = 1;

  /// Opens (or creates) the database. Tests pass an ffi [factory] and
  /// `inMemoryDatabasePath`.
  static Future<LocalDb> open({DatabaseFactory? factory, String? path}) async {
    final f = factory ?? databaseFactory;
    final dbPath = path ?? p.join(await f.getDatabasesPath(), 'securepatrol.db');
    final db = await f.openDatabase(
      dbPath,
      options: OpenDatabaseOptions(version: _version, onCreate: _create),
    );
    return LocalDb._(db);
  }

  static Future<void> _create(Database db, int version) async {
    await db.execute('''
      CREATE TABLE outbox (
        id          TEXT PRIMARY KEY,
        kind        TEXT NOT NULL,
        method      TEXT NOT NULL,
        path        TEXT NOT NULL,
        body        TEXT NOT NULL,
        guard_id    TEXT,
        photos      TEXT NOT NULL DEFAULT '[]',
        priority    INTEGER NOT NULL DEFAULT 1,
        label       TEXT NOT NULL DEFAULT '',
        status      TEXT NOT NULL DEFAULT 'pending',
        attempts    INTEGER NOT NULL DEFAULT 0,
        last_error  TEXT,
        next_at     TEXT NOT NULL,
        created_at  TEXT NOT NULL
      )
    ''');
    await db.execute('CREATE INDEX idx_outbox_next ON outbox(status, priority, created_at)');
    await db.execute('''
      CREATE TABLE kv (
        key        TEXT PRIMARY KEY,
        value      TEXT NOT NULL,
        updated_at TEXT NOT NULL
      )
    ''');
  }

  // ── key/value cache ────────────────────────────────────────────────────────

  Future<void> put(String key, Object? value) async {
    if (value == null) {
      await db.delete('kv', where: 'key = ?', whereArgs: [key]);
      return;
    }
    await db.insert(
      'kv',
      {'key': key, 'value': jsonEncode(value), 'updated_at': DateTime.now().toUtc().toIso8601String()},
      conflictAlgorithm: ConflictAlgorithm.replace,
    );
  }

  Future<Object?> get(String key) async {
    final rows = await db.query('kv', columns: ['value'], where: 'key = ?', whereArgs: [key], limit: 1);
    if (rows.isEmpty) return null;
    return jsonDecode(rows.first['value'] as String);
  }

  Future<Map<String, dynamic>?> getMap(String key) async {
    final v = await get(key);
    return v is Map ? v.cast<String, dynamic>() : null;
  }

  Future<List<Map<String, dynamic>>> getList(String key) async {
    final v = await get(key);
    return v is List ? v.whereType<Map<dynamic, dynamic>>().map((e) => e.cast<String, dynamic>()).toList() : [];
  }

  /// Removes every cached value whose key starts with [prefix].
  Future<void> deletePrefix(String prefix) => db.delete('kv', where: 'key LIKE ?', whereArgs: ['$prefix%']);

  Future<void> wipe() async {
    await db.delete('kv');
    await db.delete('outbox');
  }
}

/// Cache keys.
class CacheKeys {
  static const device = 'device';
  static const site = 'site';
  static const siteConfig = 'site_config';
  static const roster = 'roster';
  static const routes = 'routes';
  static const lastSync = 'last_sync';
  static String guardStatus(String guardId) => 'guard_status:$guardId';
  static String patrol(String guardId) => 'patrol:$guardId';
  static String clock(String guardId) => 'clock:$guardId';
  static const visitors = 'visitors';
}
