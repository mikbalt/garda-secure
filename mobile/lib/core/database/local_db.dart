import 'dart:convert';
import 'dart:math';

import 'package:flutter/foundation.dart';
import 'package:path/path.dart' as p;
import 'package:sqflite/sqflite.dart';

import '../storage/field_cipher.dart';

/// On-device SQLite storage:
/// - `outbox`: every field action waiting to be sent (see [Outbox]).
/// - `kv`: JSON caches (device, site, roster, routes, per-guard state, …) so
///   the tablet keeps working offline.
///
/// Sensitive content is encrypted with [cipher] (MOB-6): every `kv` value and
/// the outbox `body`, `label` and `guard_token` columns. Columns the queue
/// needs for ordering (kind, status, priority, seq, times) stay plain.
class LocalDb {
  LocalDb._(this.db, this.cipher);

  final Database db;
  final FieldCipher cipher;

  /// True when the database could not be decrypted on open (the key in secure
  /// storage was lost) and its unreadable content was dropped.
  bool keyReset = false;

  /// 1: initial schema. 2: outbox `seq` (monotonic order), `guard_token`,
  /// `last_code`; encrypted fields.
  static const _version = 2;

  static const _keyCheck = '_key_check';

  /// Opens (or creates) the database. Tests pass an ffi [factory] and
  /// `inMemoryDatabasePath`; without a [cipher] an ephemeral key is used.
  static Future<LocalDb> open({DatabaseFactory? factory, String? path, FieldCipher? cipher}) async {
    final f = factory ?? databaseFactory;
    final dbPath = path ?? p.join(await f.getDatabasesPath(), 'securepatrol.db');
    final db = await f.openDatabase(
      dbPath,
      options: OpenDatabaseOptions(version: _version, onCreate: _create, onUpgrade: _upgrade),
    );
    final rnd = Random.secure();
    final local = LocalDb._(db, cipher ?? FieldCipher(List<int>.generate(32, (_) => rnd.nextInt(256))));
    await local._checkKey();
    await local._sealLegacy();
    return local;
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
        created_at  TEXT NOT NULL,
        seq         INTEGER NOT NULL DEFAULT 0,
        guard_token TEXT,
        last_code   TEXT
      )
    ''');
    await db.execute('CREATE INDEX idx_outbox_seq ON outbox(status, priority, seq)');
    await db.execute('''
      CREATE TABLE kv (
        key        TEXT PRIMARY KEY,
        value      TEXT NOT NULL,
        updated_at TEXT NOT NULL
      )
    ''');
  }

  static Future<void> _upgrade(Database db, int from, int to) async {
    if (from < 2) {
      // Order by a local sequence instead of the (device-clock) creation
      // time (MOB-4); existing rows keep their insertion order via rowid.
      await db.execute('ALTER TABLE outbox ADD COLUMN seq INTEGER NOT NULL DEFAULT 0');
      await db.execute('UPDATE outbox SET seq = rowid');
      await db.execute('ALTER TABLE outbox ADD COLUMN guard_token TEXT');
      await db.execute('ALTER TABLE outbox ADD COLUMN last_code TEXT');
      await db.execute('DROP INDEX IF EXISTS idx_outbox_next');
      await db.execute('CREATE INDEX idx_outbox_seq ON outbox(status, priority, seq)');
    }
  }

  /// Detects a lost key: unreadable data cannot be recovered, so it is
  /// dropped (and reported via [keyReset]) instead of failing forever.
  Future<void> _checkKey() async {
    final rows = await db.query('kv', columns: ['value'], where: 'key = ?', whereArgs: [_keyCheck]);
    if (rows.isNotEmpty) {
      try {
        if (cipher.open(rows.first['value'] as String) == 'ok') return;
      } on CipherError {
        // fall through
      }
      keyReset = true;
      debugPrint('LocalDb: database key changed; dropping unreadable data');
      await db.delete('kv');
      await db.delete('outbox');
    }
    await db.insert('kv', {'key': _keyCheck, 'value': cipher.seal('ok'), 'updated_at': _nowIso()},
        conflictAlgorithm: ConflictAlgorithm.replace);
  }

  /// Encrypts values written by an older app version (plaintext) in place.
  Future<void> _sealLegacy() async {
    final kv = await db.query('kv', columns: ['key', 'value'], where: "value NOT LIKE 'e1:%'");
    for (final r in kv) {
      await db.update('kv', {'value': cipher.seal(r['value'] as String)}, where: 'key = ?', whereArgs: [r['key']]);
    }
    final ob = await db.query('outbox', columns: ['id', 'body', 'label', 'guard_token'], where: "body NOT LIKE 'e1:%'");
    for (final r in ob) {
      await db.update(
        'outbox',
        {
          'body': cipher.seal(r['body'] as String),
          'label': cipher.seal((r['label'] as String?) ?? ''),
          'guard_token': cipher.sealNullable(r['guard_token'] as String?),
        },
        where: 'id = ?',
        whereArgs: [r['id']],
      );
    }
  }

  static String _nowIso() => DateTime.now().toUtc().toIso8601String();

  // ── key/value cache ────────────────────────────────────────────────────────

  Future<void> put(String key, Object? value) async {
    if (value == null) {
      await db.delete('kv', where: 'key = ?', whereArgs: [key]);
      return;
    }
    await db.insert(
      'kv',
      {'key': key, 'value': cipher.seal(jsonEncode(value)), 'updated_at': _nowIso()},
      conflictAlgorithm: ConflictAlgorithm.replace,
    );
  }

  Future<Object?> get(String key) async {
    final rows = await db.query('kv', columns: ['value'], where: 'key = ?', whereArgs: [key], limit: 1);
    if (rows.isEmpty) return null;
    try {
      return jsonDecode(cipher.open(rows.first['value'] as String));
    } on CipherError {
      return null;
    } on FormatException {
      return null;
    }
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

  // ── outbox rows ────────────────────────────────────────────────────────────

  /// Inserts an outbox row with the next local sequence number (FIFO order
  /// independent of the device clock).
  Future<void> insertOutbox(Map<String, Object?> row) => db.transaction((tx) async {
        final r = await tx.rawQuery('SELECT COALESCE(MAX(seq), 0) + 1 AS n FROM outbox');
        await tx.insert('outbox', {...row, 'seq': (r.first['n'] as num).toInt()});
      });

  Future<int> outboxCount() async {
    final r = await db.rawQuery('SELECT COUNT(*) AS n FROM outbox');
    return (r.first['n'] as num).toInt();
  }

  /// Deletes every cache value. Refuses while the outbox still holds items:
  /// unsent field data is never dropped silently (MOB-2).
  Future<void> wipe() async {
    if (await outboxCount() > 0) throw StateError('outbox not empty');
    await db.delete('kv', where: 'key <> ?', whereArgs: [_keyCheck]);
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

  /// When the roster was last synced successfully (MOB-6 max offline age).
  static const rosterSyncedAt = 'roster_synced_at';

  /// Server − device clock offset in ms (MOB-4).
  static const clockOffset = 'clock_offset_ms';

  /// Persistent PIN lockout state per guard (MOB-6).
  static const pinLockout = 'pin_lockout';

  /// Local error log ring buffer (MOB-7).
  static const errorLog = 'error_log';

  static String guardStatus(String guardId) => 'guard_status:$guardId';
  static String patrol(String guardId) => 'patrol:$guardId';
  static String clock(String guardId) => 'clock:$guardId';
  static const visitors = 'visitors';
}
