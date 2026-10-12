import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:connectivity_plus/connectivity_plus.dart';
import 'package:flutter/foundation.dart';

import '../api/api_client.dart';
import 'local_db.dart';

/// A photo attached to an outbox operation. It is uploaded (with its client
/// ID, so the object key is deterministic) before the operation is sent.
class OutboxPhoto {
  const OutboxPhoto({required this.id, required this.kind, required this.path, this.uploaded = false});

  final String id;
  final String kind;
  final String path;
  final bool uploaded;

  OutboxPhoto done() => OutboxPhoto(id: id, kind: kind, path: path, uploaded: true);

  Map<String, dynamic> toJson() => {'id': id, 'kind': kind, 'path': path, 'uploaded': uploaded};

  factory OutboxPhoto.fromJson(Map<String, dynamic> j) =>
      OutboxPhoto(id: j['id'] as String, kind: j['kind'] as String, path: j['path'] as String, uploaded: j['uploaded'] == true);
}

/// One queued API call.
class OutboxOp {
  OutboxOp({
    required this.id,
    required this.kind,
    required this.method,
    required this.path,
    required this.body,
    this.guardId,
    this.photos = const [],
    this.priority = 1,
    this.label = '',
    this.status = 'pending',
    this.attempts = 0,
    this.lastError,
    DateTime? nextAt,
    DateTime? createdAt,
  })  : nextAt = nextAt ?? DateTime.now().toUtc(),
        createdAt = createdAt ?? DateTime.now().toUtc();

  final String id;

  /// clock_in, clock_out, session_start, scan, session_end, incident, defect,
  /// visitor, visitor_exit, sos, sos_ack.
  final String kind;
  final String method;
  final String path;
  final Map<String, dynamic> body;
  final String? guardId;
  final List<OutboxPhoto> photos;

  /// 0 = SOS (always first), 1 = everything else (strict FIFO).
  final int priority;

  /// Short human description shown in the outbox screen.
  final String label;
  final String status; // pending | failed
  final int attempts;
  final String? lastError;
  final DateTime nextAt;
  final DateTime createdAt;

  Map<String, Object?> toRow() => {
        'id': id,
        'kind': kind,
        'method': method,
        'path': path,
        'body': jsonEncode(body),
        'guard_id': guardId,
        'photos': jsonEncode(photos.map((p) => p.toJson()).toList()),
        'priority': priority,
        'label': label,
        'status': status,
        'attempts': attempts,
        'last_error': lastError,
        'next_at': nextAt.toIso8601String(),
        'created_at': createdAt.toIso8601String(),
      };

  factory OutboxOp.fromRow(Map<String, Object?> r) => OutboxOp(
        id: r['id'] as String,
        kind: r['kind'] as String,
        method: r['method'] as String,
        path: r['path'] as String,
        body: (jsonDecode(r['body'] as String) as Map).cast<String, dynamic>(),
        guardId: r['guard_id'] as String?,
        photos: (jsonDecode(r['photos'] as String) as List).map((e) => OutboxPhoto.fromJson((e as Map).cast<String, dynamic>())).toList(),
        priority: (r['priority'] as num).toInt(),
        label: r['label'] as String? ?? '',
        status: r['status'] as String,
        attempts: (r['attempts'] as num).toInt(),
        lastError: r['last_error'] as String?,
        nextAt: DateTime.parse(r['next_at'] as String),
        createdAt: DateTime.parse(r['created_at'] as String),
      );
}

/// Persistent send queue (D-024). Every field action is written here first and
/// sent in order when the server is reachable. Requests are idempotent on the
/// server (client UUIDs, D-006), so resending after a lost response is safe.
class Outbox extends ChangeNotifier {
  Outbox(this._db, this._api, {this.onAuthExpired, Stream<List<ConnectivityResult>>? connectivity, this.tick = const Duration(seconds: 5)})
      : _connectivity = connectivity;

  final LocalDb _db;
  final ApiClient _api;
  final Stream<List<ConnectivityResult>>? _connectivity;
  final Duration tick;

  /// Called when the device token can no longer be refreshed (revoked).
  final VoidCallback? onAuthExpired;

  int pending = 0;
  int failed = 0;
  bool sosPending = false;

  /// An SOS was rejected by the server and is parked in the outbox (only
  /// SOS queued by older app versions; new ones are never parked).
  bool sosFailed = false;

  /// The last attempt to send an SOS was rejected by the server (not a
  /// network problem): it keeps retrying, but the guard must call for help.
  bool sosRejected = false;

  /// Attaches late information (e.g. GPS) to an operation that has not been
  /// sent yet. No-op once it was sent.
  Future<void> amendBody(String id, Map<String, dynamic> extra) async {
    final rows = await _db.db.query('outbox', columns: ['body'], where: "id = ? AND status = 'pending'", whereArgs: [id]);
    if (rows.isEmpty) return;
    final body = (jsonDecode(rows.first['body'] as String) as Map).cast<String, dynamic>()..addAll(extra);
    await _db.db.update('outbox', {'body': jsonEncode(body)}, where: "id = ? AND status = 'pending'", whereArgs: [id]);
  }
  bool online = true;
  DateTime? lastSuccess;

  Timer? _timer;
  StreamSubscription<List<ConnectivityResult>>? _sub;
  Future<void>? _running;
  bool _disposed = false;

  static const _retryDelays = [5, 15, 30, 60, 120, 300, 600];

  /// Delay before the next attempt of an operation that failed `attempts` times.
  static Duration backoff(String kind, int attempts) {
    if (kind == 'sos') return const Duration(seconds: 10);
    final i = (attempts - 1).clamp(0, _retryDelays.length - 1);
    return Duration(seconds: _retryDelays[i]);
  }

  Future<void> start() async {
    await refreshCounts();
    final ls = await _db.get(CacheKeys.lastSync);
    if (ls is String) lastSuccess = DateTime.tryParse(ls);
    _timer = Timer.periodic(tick, (_) => process());
    _sub = (_connectivity ?? Connectivity().onConnectivityChanged).listen((results) {
      final up = results.any((r) => r != ConnectivityResult.none);
      if (up) {
        // Back online: retry now instead of waiting out the backoff (up to 10 min).
        unawaited(flush());
      } else {
        _setOnline(false);
      }
    });
    unawaited(process());
  }

  /// Stores [op]; with [send] (default) also starts sending right away.
  Future<void> enqueue(OutboxOp op, {bool send = true}) async {
    await _db.db.insert('outbox', op.toRow());
    await refreshCounts();
    if (send) unawaited(process());
  }

  Future<List<OutboxOp>> list() async {
    final rows = await _db.db.query('outbox', orderBy: 'priority, created_at');
    return rows.map(OutboxOp.fromRow).toList();
  }

  Future<void> retry(String id) async {
    await _db.db.update('outbox', {'status': 'pending', 'next_at': DateTime.now().toUtc().toIso8601String()}, where: 'id = ?', whereArgs: [id]);
    await refreshCounts();
    unawaited(process());
  }

  Future<void> discard(String id) async {
    await _db.db.delete('outbox', where: 'id = ?', whereArgs: [id]);
    await refreshCounts();
  }

  /// Retries everything now (e.g. "Send now" button).
  Future<void> flush() async {
    await _db.db.update('outbox', {'next_at': DateTime.now().toUtc().toIso8601String()}, where: "status = 'pending'");
    await process();
  }

  Future<void> refreshCounts() async {
    final rows = await _db.db.rawQuery(
        "SELECT status, COUNT(*) AS n, SUM(CASE WHEN kind = 'sos' THEN 1 ELSE 0 END) AS sos FROM outbox GROUP BY status");
    var p = 0, f = 0, sos = 0, sosF = 0;
    for (final r in rows) {
      final n = (r['n'] as num).toInt();
      if (r['status'] == 'pending') {
        p = n;
        sos = (r['sos'] as num? ?? 0).toInt();
      } else {
        f = n;
        sosF = (r['sos'] as num? ?? 0).toInt();
      }
    }
    pending = p;
    failed = f;
    sosPending = sos > 0;
    sosFailed = sosF > 0;
    if (!_disposed) notifyListeners();
  }

  /// Sends due operations in order. Only one run at a time.
  Future<void> process() => _running ??= _process().whenComplete(() => _running = null);

  Future<void> _process() async {
    while (!_disposed) {
      final rows = await _db.db.query('outbox', where: "status = 'pending'", orderBy: 'priority, created_at', limit: 1);
      if (rows.isEmpty) break;
      var op = OutboxOp.fromRow(rows.first);
      // Strict FIFO: never overtake an earlier operation that is waiting.
      if (op.nextAt.isAfter(DateTime.now().toUtc())) break;

      try {
        op = await _uploadPhotos(op);
        await _api.send(op.method, op.path, body: op.body, guardId: op.guardId);
        await _db.db.delete('outbox', where: 'id = ?', whereArgs: [op.id]);
        if (op.kind == 'sos') sosRejected = false;
        for (final ph in op.photos) {
          try {
            File(ph.path).deleteSync(); // sent: free the tablet's storage
          } catch (_) {}
        }
        lastSuccess = DateTime.now().toUtc();
        await _db.put(CacheKeys.lastSync, lastSuccess!.toIso8601String());
        _setOnline(true);
      } on ApiException catch (e) {
        if (e.code == 'auth_expired') {
          onAuthExpired?.call();
          break;
        }
        if (e.retryable) {
          await _db.db.update(
            'outbox',
            {
              'attempts': op.attempts + 1,
              'last_error': e.message,
              'next_at': DateTime.now().toUtc().add(backoff(op.kind, op.attempts + 1)).toIso8601String(),
            },
            where: 'id = ?',
            whereArgs: [op.id],
          );
          if (e.network) _setOnline(false);
          await refreshCounts();
          break;
        }
        if (op.kind == 'sos') {
          // An SOS is never parked. A skewed device clock is the likely cause
          // of a rejection: drop the client time and let the server stamp it.
          final body = Map<String, dynamic>.of(op.body);
          final skew = e.code == 'timestamp_out_of_range' && body.remove('fired_at') != null;
          sosRejected = !skew;
          await _db.db.update(
            'outbox',
            {
              'body': jsonEncode(body),
              'attempts': op.attempts + 1,
              'last_error': e.code != null ? '${e.message} (${e.code})' : e.message,
              'next_at': (skew ? DateTime.now().toUtc() : DateTime.now().toUtc().add(backoff('sos', op.attempts + 1))).toIso8601String(),
            },
            where: 'id = ?',
            whereArgs: [op.id],
          );
          _setOnline(true);
          await refreshCounts();
          if (skew) continue;
          break;
        }
        // Rejected by the server (validation, conflict): park it and go on.
        await _db.db.update(
          'outbox',
          {'status': 'failed', 'attempts': op.attempts + 1, 'last_error': e.code != null ? '${e.message} (${e.code})' : e.message},
          where: 'id = ?',
          whereArgs: [op.id],
        );
        _setOnline(true);
      }
      await refreshCounts();
    }
  }

  Future<OutboxOp> _uploadPhotos(OutboxOp op) async {
    if (op.photos.every((p) => p.uploaded)) return op;
    final photos = <OutboxPhoto>[];
    for (final ph in op.photos) {
      if (ph.uploaded || !File(ph.path).existsSync()) {
        photos.add(ph.done());
        continue;
      }
      await _api.uploadPhoto(filePath: ph.path, id: ph.id, kind: ph.kind, guardId: op.guardId);
      photos.add(ph.done());
      await _db.db.update('outbox', {'photos': jsonEncode(photos.map((p) => p.toJson()).toList() + op.photos.skip(photos.length).map((p) => p.toJson()).toList())},
          where: 'id = ?', whereArgs: [op.id]);
    }
    return OutboxOp(
      id: op.id,
      kind: op.kind,
      method: op.method,
      path: op.path,
      body: op.body,
      guardId: op.guardId,
      photos: photos,
      priority: op.priority,
      label: op.label,
      status: op.status,
      attempts: op.attempts,
      lastError: op.lastError,
      nextAt: op.nextAt,
      createdAt: op.createdAt,
    );
  }

  void _setOnline(bool v) {
    if (online == v) return;
    online = v;
    if (!_disposed) notifyListeners();
  }

  @override
  void dispose() {
    _disposed = true;
    _timer?.cancel();
    _sub?.cancel();
    super.dispose();
  }
}
