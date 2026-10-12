import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:connectivity_plus/connectivity_plus.dart';
import 'package:flutter/foundation.dart';

import '../api/api_client.dart';
import '../storage/field_cipher.dart';
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
    this.guardToken,
    this.photos = const [],
    this.priority = 1,
    this.label = '',
    this.status = 'pending',
    this.attempts = 0,
    this.lastError,
    this.lastCode,
    this.seq = 0,
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

  /// Guard session token valid when the action was taken (D-035); sent as
  /// `X-Guard-Token`. Null when the guard unlocked offline.
  final String? guardToken;
  final List<OutboxPhoto> photos;

  /// 0 = SOS (always first), 1 = everything else (strict FIFO).
  final int priority;

  /// Short human description shown in the outbox screen.
  final String label;
  final String status; // pending | failed
  final int attempts;
  final String? lastError;

  /// Machine code of the last failure (server code or a local one such as
  /// `photo_missing`, `retry_limit`), translated in the outbox screen.
  final String? lastCode;

  /// Local insertion order (monotonic; independent of the device clock).
  final int seq;
  final DateTime nextAt;

  /// Device time when queued (display and age cap only).
  final DateTime createdAt;

  Map<String, Object?> toRow(FieldCipher c) => {
        'id': id,
        'kind': kind,
        'method': method,
        'path': path,
        'body': c.seal(jsonEncode(body)),
        'guard_id': guardId,
        'guard_token': c.sealNullable(guardToken),
        'photos': jsonEncode(photos.map((p) => p.toJson()).toList()),
        'priority': priority,
        'label': c.seal(label),
        'status': status,
        'attempts': attempts,
        'last_error': lastError,
        'last_code': lastCode,
        'next_at': nextAt.toIso8601String(),
        'created_at': createdAt.toIso8601String(),
      };

  factory OutboxOp.fromRow(Map<String, Object?> r, FieldCipher c) => OutboxOp(
        id: r['id'] as String,
        kind: r['kind'] as String,
        method: r['method'] as String,
        path: r['path'] as String,
        body: (jsonDecode(c.open(r['body'] as String)) as Map).cast<String, dynamic>(),
        guardId: r['guard_id'] as String?,
        guardToken: c.openNullable(r['guard_token'] as String?),
        photos: (jsonDecode(r['photos'] as String) as List).map((e) => OutboxPhoto.fromJson((e as Map).cast<String, dynamic>())).toList(),
        priority: (r['priority'] as num).toInt(),
        label: c.open((r['label'] as String?) ?? ''),
        status: r['status'] as String,
        attempts: (r['attempts'] as num).toInt(),
        lastError: r['last_error'] as String?,
        lastCode: r['last_code'] as String?,
        seq: (r['seq'] as num?)?.toInt() ?? 0,
        nextAt: DateTime.parse(r['next_at'] as String),
        createdAt: DateTime.parse(r['created_at'] as String),
      );

  OutboxOp withPhotos(List<OutboxPhoto> p) => OutboxOp(
        id: id,
        kind: kind,
        method: method,
        path: path,
        body: body,
        guardId: guardId,
        guardToken: guardToken,
        photos: p,
        priority: priority,
        label: label,
        status: status,
        attempts: attempts,
        lastError: lastError,
        lastCode: lastCode,
        seq: seq,
        nextAt: nextAt,
        createdAt: createdAt,
      );
}

class _PhotoMissing implements Exception {
  const _PhotoMissing();
}

/// Persistent send queue (D-024). Every field action is written here first and
/// sent in order when the server is reachable. Requests are idempotent on the
/// server (client UUIDs, D-006), so resending after a lost response is safe.
///
/// Failure handling (MOB-5):
/// - network errors retry with backoff and never give up;
/// - server errors (5xx) retry with backoff, but a non-SOS item that still
///   fails after [maxAttempts] or [maxAge] is parked as failed (`retry_limit`)
///   so one poison item cannot block the queue forever;
/// - 429 waits for `Retry-After`;
/// - "already done" answers to a replay count as success;
/// - device-level 403s (`device_pending`, `device_revoked`, …) pause the whole
///   queue without touching the items and tell the session;
/// - 403 `guard_token_invalid` drops the token and resends at once;
/// - other rejections park the item (visible to the guard, translated).
/// An SOS is never parked.
class Outbox extends ChangeNotifier {
  Outbox(
    this._db,
    this._api, {
    this.onAuthExpired,
    this.onDeviceDenied,
    this.onGuardTokenInvalid,
    Stream<List<ConnectivityResult>>? connectivity,
    this.tick = const Duration(seconds: 5),
    this.maxAttempts = 50,
    this.maxAge = const Duration(days: 3),
  }) : _connectivity = connectivity;

  final LocalDb _db;
  final ApiClient _api;
  final Stream<List<ConnectivityResult>>? _connectivity;
  final Duration tick;
  final int maxAttempts;
  final Duration maxAge;

  /// Called when the device token can no longer be refreshed (revoked).
  final VoidCallback? onAuthExpired;

  /// Called with the code of a device-level denial (queue paused).
  final void Function(String code)? onDeviceDenied;

  /// Called when the server rejected a guard's session token.
  final void Function(String guardId)? onGuardTokenInvalid;

  int pending = 0;
  int failed = 0;
  bool sosPending = false;

  /// An SOS was rejected by the server and is parked in the outbox (only
  /// SOS queued by older app versions; new ones are never parked).
  bool sosFailed = false;

  /// The last attempt to send an SOS was rejected by the server (not a
  /// network problem): it keeps retrying, but the guard must call for help.
  bool sosRejected = false;

  bool online = true;
  DateTime? lastSuccess;

  /// Sending is paused because the server refused the device (MOB-5).
  String? pauseCode;
  DateTime? _pausedUntil;

  /// How long a device-level denial pauses the queue before one probe.
  static const pauseFor = Duration(seconds: 60);

  Timer? _timer;
  StreamSubscription<List<ConnectivityResult>>? _sub;
  Future<void>? _running;
  bool _disposed = false;

  static const _retryDelays = [5, 15, 30, 60, 120, 300, 600];

  /// Device-level denials: nothing can be sent until an admin acts.
  static const deviceDenials = {'device_pending', 'device_revoked', 'device_inactive', 'device_unregistered'};

  /// Delay before the next attempt of an operation that failed `attempts` times.
  static Duration backoff(String kind, int attempts) {
    if (kind == 'sos') return const Duration(seconds: 10);
    final i = (attempts - 1).clamp(0, _retryDelays.length - 1);
    return Duration(seconds: _retryDelays[i]);
  }

  static String _now() => DateTime.now().toUtc().toIso8601String();

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
    await _db.insertOutbox(op.toRow(_db.cipher));
    await refreshCounts();
    if (send) unawaited(process());
  }

  OutboxOp? _read(Map<String, Object?> row) {
    try {
      return OutboxOp.fromRow(row, _db.cipher);
    } catch (_) {
      return null;
    }
  }

  Future<List<OutboxOp>> list() async {
    final rows = await _db.db.query('outbox', orderBy: 'priority, seq');
    return rows.map(_read).whereType<OutboxOp>().toList();
  }

  Future<int> count() => _db.outboxCount();

  /// Attaches late information (e.g. GPS) to an operation that has not been
  /// sent yet. No-op once it was sent.
  Future<void> amendBody(String id, Map<String, dynamic> extra) async {
    final rows = await _db.db.query('outbox', columns: ['body'], where: "id = ? AND status = 'pending'", whereArgs: [id]);
    if (rows.isEmpty) return;
    final body = (jsonDecode(_db.cipher.open(rows.first['body'] as String)) as Map).cast<String, dynamic>()..addAll(extra);
    await _db.db.update('outbox', {'body': _db.cipher.seal(jsonEncode(body))}, where: "id = ? AND status = 'pending'", whereArgs: [id]);
  }

  /// Manual retry of a parked item: a fresh budget of attempts.
  Future<void> retry(String id) async {
    await _db.db.update('outbox', {'status': 'pending', 'attempts': 0, 'next_at': _now()}, where: 'id = ?', whereArgs: [id]);
    await refreshCounts();
    unawaited(process());
  }

  /// Removes an item (the UI asks for a supervisor first, MOB-2) and its
  /// photo files.
  Future<void> discard(String id) async {
    final rows = await _db.db.query('outbox', columns: ['photos'], where: 'id = ?', whereArgs: [id]);
    await _db.db.delete('outbox', where: 'id = ?', whereArgs: [id]);
    for (final r in rows) {
      _deleteFiles((jsonDecode(r['photos'] as String) as List).map((e) => OutboxPhoto.fromJson((e as Map).cast<String, dynamic>())));
    }
    await refreshCounts();
  }

  /// Retries everything now (e.g. "Send now" button, back online).
  Future<void> flush() async {
    _pausedUntil = null;
    await _db.db.update('outbox', {'next_at': _now()}, where: "status = 'pending'");
    await process();
  }

  /// Lifts a device-level pause (the session saw the device active again).
  void resume() {
    if (pauseCode == null && _pausedUntil == null) return;
    pauseCode = null;
    _pausedUntil = null;
    if (!_disposed) notifyListeners();
    unawaited(process());
  }

  /// Photo files still referenced by queued items (for orphan cleanup).
  Future<Set<String>> photoPaths() async {
    final rows = await _db.db.query('outbox', columns: ['photos']);
    return {
      for (final r in rows)
        for (final p in (jsonDecode(r['photos'] as String) as List)) ((p as Map)['path'] as String),
    };
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
      final until = _pausedUntil;
      if (until != null && DateTime.now().isBefore(until)) break;
      final rows = await _db.db.query('outbox', where: "status = 'pending'", orderBy: 'priority, seq', limit: 1);
      if (rows.isEmpty) break;
      final read = _read(rows.first);
      if (read == null) {
        // Undecryptable row (should not happen): park it instead of looping.
        await _db.db.update('outbox', {'status': 'failed', 'last_code': 'unreadable', 'last_error': 'unreadable'}, where: 'id = ?', whereArgs: [rows.first['id']]);
        await refreshCounts();
        continue;
      }
      var op = read;
      // Strict FIFO: never overtake an earlier operation that is waiting.
      if (op.nextAt.isAfter(DateTime.now().toUtc())) break;

      try {
        op = await _uploadPhotos(op);
        await _api.send(op.method, op.path, body: op.body, guardId: op.guardId, guardToken: op.guardToken);
        await _done(op);
      } on _PhotoMissing {
        // The photo file is gone (storage cleared?): it can never be sent.
        await _park(op, code: 'photo_missing', message: 'photo file missing on the tablet');
      } on ApiException catch (e) {
        if (e.code == 'auth_expired') {
          onAuthExpired?.call();
          break;
        }
        if (e.code == 'guard_token_invalid' && op.guardToken != null) {
          // Stale/forged-looking token: send again as an unverified action.
          await _db.db.update('outbox', {'guard_token': null}, where: 'id = ?', whereArgs: [op.id]);
          if (op.guardId != null) onGuardTokenInvalid?.call(op.guardId!);
          continue;
        }
        if (e.status == 403 && deviceDenials.contains(e.code) || e.code == 'device_unregistered') {
          // Not the item's fault: pause everything, keep the items as they are.
          pauseCode = e.code;
          _pausedUntil = DateTime.now().add(pauseFor);
          if (op.kind == 'sos') sosRejected = true;
          _setOnline(true);
          onDeviceDenied?.call(e.code!);
          if (!_disposed) notifyListeners();
          break;
        }
        if (await _alreadyDone(op, e)) {
          await _done(op);
          continue;
        }
        if (e.status == 429) {
          await _reschedule(op, e, e.retryAfter ?? const Duration(seconds: 30));
          _setOnline(true);
          break;
        }
        if (e.retryable) {
          if (op.kind != 'sos' && !e.network && _exhausted(op)) {
            await _park(op, code: 'retry_limit', message: e.message);
            continue;
          }
          await _reschedule(op, e, backoff(op.kind, op.attempts + 1));
          if (e.network) _setOnline(false);
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
              'body': _db.cipher.seal(jsonEncode(body)),
              'attempts': op.attempts + 1,
              'last_error': e.message,
              'last_code': e.code,
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
        await _park(op, code: e.code, message: e.message);
        _setOnline(true);
      }
    }
    await refreshCounts();
  }

  bool _exhausted(OutboxOp op) =>
      op.attempts + 1 >= maxAttempts || (op.attempts >= 10 && DateTime.now().toUtc().difference(op.createdAt) > maxAge);

  Future<void> _reschedule(OutboxOp op, ApiException e, Duration wait) async {
    await _db.db.update(
      'outbox',
      {
        'attempts': op.attempts + 1,
        'last_error': e.message,
        'last_code': e.code ?? (e.network ? 'network' : null),
        'next_at': DateTime.now().toUtc().add(wait).toIso8601String(),
      },
      where: 'id = ?',
      whereArgs: [op.id],
    );
    await refreshCounts();
  }

  Future<void> _park(OutboxOp op, {String? code, required String message}) async {
    await _db.db.update(
      'outbox',
      {'status': 'failed', 'attempts': op.attempts + 1, 'last_error': message, 'last_code': code},
      where: 'id = ?',
      whereArgs: [op.id],
    );
    await refreshCounts();
  }

  Future<void> _done(OutboxOp op) async {
    await _db.db.delete('outbox', where: 'id = ?', whereArgs: [op.id]);
    if (op.kind == 'sos') sosRejected = false;
    _deleteFiles(op.photos); // sent: free the tablet's storage
    lastSuccess = DateTime.now().toUtc();
    await _db.put(CacheKeys.lastSync, lastSuccess!.toIso8601String());
    _setOnline(true);
    await refreshCounts();
  }

  static void _deleteFiles(Iterable<OutboxPhoto> photos) {
    for (final ph in photos) {
      try {
        File(ph.path).deleteSync();
      } catch (_) {}
    }
  }

  /// A replayed action the server reports as already applied is a success
  /// (e.g. the response of the first attempt was lost).
  Future<bool> _alreadyDone(OutboxOp op, ApiException e) async {
    switch ((op.kind, e.code)) {
      case ('clock_out', 'not_clocked_in'):
        // Unless our own clock-in never made it (then the guard must see it).
        final ops = await list();
        return !ops.any((o) => o.kind == 'clock_in' && o.guardId == op.guardId && o.status == 'failed');
      case ('clock_in', 'already_clocked_in'):
        // The guard is clocked in (earlier attempt or another tablet). Later
        // clock-outs must close the server's record, not our client ID.
        final attendanceId = op.body['id'];
        for (final o in await list()) {
          if (o.kind == 'clock_out' && o.guardId == op.guardId && o.body['attendance_id'] == attendanceId) {
            final body = Map<String, dynamic>.of(o.body)..remove('attendance_id');
            await _db.db.update('outbox', {'body': _db.cipher.seal(jsonEncode(body))}, where: 'id = ?', whereArgs: [o.id]);
          }
        }
        return true;
      case ('session_end', 'session_ended'):
      case ('sos_ack', 'already_acknowledged'):
      case ('visitor_exit', 'already_exited'):
        return true;
    }
    return false;
  }

  Future<OutboxOp> _uploadPhotos(OutboxOp op) async {
    if (op.photos.every((p) => p.uploaded)) return op;
    final photos = List<OutboxPhoto>.of(op.photos);
    for (var i = 0; i < photos.length; i++) {
      final ph = photos[i];
      if (ph.uploaded) continue;
      if (!File(ph.path).existsSync()) throw const _PhotoMissing();
      try {
        await _api.uploadPhoto(filePath: ph.path, id: ph.id, kind: ph.kind, guardId: op.guardId, guardToken: op.guardToken);
      } on ApiException catch (e) {
        // Write-once evidence: the server already has this photo.
        if (e.code != 'media_exists') rethrow;
      }
      photos[i] = ph.done();
      await _db.db.update('outbox', {'photos': jsonEncode(photos.map((p) => p.toJson()).toList())}, where: 'id = ?', whereArgs: [op.id]);
    }
    return op.withPhotos(photos);
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
