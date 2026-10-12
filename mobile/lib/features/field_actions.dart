import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:uuid/uuid.dart';

import '../core/api/api_client.dart';
import '../core/auth/auth_provider.dart';
import '../core/database/local_db.dart';
import '../core/database/sync_service.dart';
import '../core/services/clock.dart';
import '../core/services/location_service.dart';
import '../core/services/photo_service.dart';
import '../shared/models/models.dart';
import 'patrol/progress.dart';

/// Local clock-in state of a guard (what this tablet recorded).
class ClockState {
  const ClockState({required this.attendanceId, required this.clockInAt, this.clockOutAt});

  final String attendanceId;
  final DateTime clockInAt;
  final DateTime? clockOutAt;

  bool get open => clockOutAt == null;

  Map<String, dynamic> toJson() => {'id': attendanceId, 'in': clockInAt.toIso8601String(), 'out': clockOutAt?.toIso8601String()};

  static ClockState? fromJson(Map<String, dynamic>? j) => j == null
      ? null
      : ClockState(
          attendanceId: j['id'] as String,
          clockInAt: DateTime.parse(j['in'] as String).toUtc(),
          clockOutAt: j['out'] == null ? null : DateTime.parse(j['out'] as String).toUtc(),
        );
}

/// The guard's running patrol as known on this tablet.
class ActivePatrol {
  const ActivePatrol({required this.sessionId, required this.route, required this.startedAt, required this.scans});

  final String sessionId;
  final PatrolRoute route;
  final DateTime startedAt;
  final List<LocalScan> scans;

  PatrolProgress progress(DateTime now) => computeProgress(route, startedAt, scans, now);

  Map<String, dynamic> toJson() => {
        'session_id': sessionId,
        'route': route.toJson(),
        'started_at': startedAt.toIso8601String(),
        'scans': scans.map((s) => s.toJson()).toList(),
      };

  static ActivePatrol? fromJson(Map<String, dynamic>? j) => j == null
      ? null
      : ActivePatrol(
          sessionId: j['session_id'] as String,
          route: PatrolRoute.fromJson((j['route'] as Map).cast<String, dynamic>()),
          startedAt: DateTime.parse(j['started_at'] as String).toUtc(),
          scans: ((j['scans'] as List?) ?? const []).map((e) => LocalScan.fromJson((e as Map).cast<String, dynamic>())).toList(),
        );
}

/// A visitor recorded on this tablet (the tablet cannot list the site's
/// visitor log, so it keeps its own to record exits).
class LocalVisitor {
  const LocalVisitor({required this.id, required this.name, required this.entryAt, this.plate, this.host, this.purpose, this.exitAt});

  final String id;
  final String name;
  final DateTime entryAt;
  final String? plate;
  final String? host;
  final String? purpose;
  final DateTime? exitAt;

  bool get inside => exitAt == null;

  LocalVisitor exited(DateTime at) => LocalVisitor(id: id, name: name, entryAt: entryAt, plate: plate, host: host, purpose: purpose, exitAt: at);

  Map<String, dynamic> toJson() => {
        'id': id,
        'name': name,
        'entry_at': entryAt.toIso8601String(),
        'plate': plate,
        'host': host,
        'purpose': purpose,
        'exit_at': exitAt?.toIso8601String(),
      };

  factory LocalVisitor.fromJson(Map<String, dynamic> j) => LocalVisitor(
        id: j['id'] as String,
        name: j['name'] as String,
        entryAt: DateTime.parse(j['entry_at'] as String).toUtc(),
        plate: j['plate'] as String?,
        host: j['host'] as String?,
        purpose: j['purpose'] as String?,
        exitAt: j['exit_at'] == null ? null : DateTime.parse(j['exit_at'] as String).toUtc(),
      );
}

/// Every field action of the acting guard. Each one updates local state for
/// immediate feedback and queues the API call in the [Outbox] (offline-first,
/// idempotent with client UUIDs and event timestamps).
class FieldActions extends ChangeNotifier {
  FieldActions({required this.session, required this.outbox, required this.db, required this.location, required this.photos});

  final DeviceSession session;
  final Outbox outbox;
  final LocalDb db;
  final LocationService location;
  final PhotoService photos;

  static const _uuid = Uuid();

  ClockState? clock;
  ActivePatrol? patrol;
  List<LocalVisitor> visitors = const [];

  /// IDs of SOS alerts raised on this tablet whose banner is still shown
  /// (waiting, rejected, or briefly "delivered").
  final Set<String> sosOutstanding = {};

  /// True while an SOS is being prepared (GPS fix) and not yet in the outbox.
  bool sosPreparing = false;
  Timer? _sosClear;

  String get _guardId => session.guard!.id;
  String get _siteId => session.site!.id;

  /// Server-corrected event time (MOB-4).
  DateTime get _now => AppClock.now();

  /// The acting guard's session token at the time of the action (D-035).
  String? get _token => session.guardToken;

  /// Loads the guard's local state after unlock, reconciled with the server.
  Future<void> loadForGuard() async {
    final g = session.guard;
    if (g == null) return;
    clock = ClockState.fromJson(await db.getMap(CacheKeys.clock(g.id)));
    patrol = ActivePatrol.fromJson(await db.getMap(CacheKeys.patrol(g.id)));
    visitors = (await db.getList(CacheKeys.visitors)).map(LocalVisitor.fromJson).where((v) => v.inside || _now.difference(v.entryAt).inHours < 24).toList();
    session.patrolRunning = patrol != null;
    notifyListeners();
    await reconcile();
  }

  /// Aligns local state with `/device/status` once the server has seen every
  /// queued action of this guard.
  Future<void> reconcile() async {
    final g = session.guard;
    final st = session.guardStatus;
    if (g == null || st == null) return;
    final ops = await outbox.list();
    final mine = ops.where((o) => o.guardId == g.id && o.status == 'pending');
    final open = st.openAttendance;
    if (!mine.any((o) => o.kind == 'clock_in' || o.kind == 'clock_out')) {
      clock = open == null ? null : ClockState(attendanceId: open.id, clockInAt: open.clockInAt!, clockOutAt: null);
      await db.put(CacheKeys.clock(g.id), clock?.toJson());
    }
    final patrolOps = mine.any((o) => o.kind == 'session_start' || o.kind == 'scan' || o.kind == 'session_end');
    if (!patrolOps) {
      if (st.sessionId == null && patrol != null) {
        patrol = null; // finished or abandoned elsewhere
        await db.put(CacheKeys.patrol(g.id), null);
      } else if (st.sessionId != null && patrol?.sessionId != st.sessionId) {
        await _restorePatrol(st.sessionId!);
      }
    }
    session.patrolRunning = patrol != null;
    notifyListeners();
  }

  Future<void> _restorePatrol(String sessionId) async {
    try {
      final res = (await session.api.get('/patrol/sessions/$sessionId', guardId: _guardId, guardToken: _token) as Map).cast<String, dynamic>();
      final s = (res['session'] as Map).cast<String, dynamic>();
      final route = session.routes.where((r) => r.id == s['route_id']).firstOrNull;
      if (route == null) return;
      final scans = <LocalScan>[];
      for (final c in (res['checkpoints'] as List? ?? const [])) {
        final m = (c as Map).cast<String, dynamic>();
        if (m['scanned_at'] is String) {
          scans.add(LocalScan(checkpointId: m['id'] as String, scannedAt: DateTime.parse(m['scanned_at'] as String).toUtc(), method: (m['method'] as String?) ?? 'qr'));
        }
      }
      patrol = ActivePatrol(sessionId: sessionId, route: route, startedAt: DateTime.parse(s['started_at'] as String).toUtc(), scans: scans);
      await db.put(CacheKeys.patrol(_guardId), patrol!.toJson());
    } on ApiException {
      // Offline: try again on the next reconcile.
    }
  }

  // ── attendance ─────────────────────────────────────────────────────────────

  Future<void> clockIn({LocalPhoto? selfie, Fix? fix}) async {
    final id = _uuid.v4();
    final at = _now;
    await outbox.enqueue(OutboxOp(
      id: _uuid.v4(),
      kind: 'clock_in',
      method: 'POST',
      path: '/attendance/clock-in',
      guardId: _guardId,
      guardToken: _token,
      label: session.guard!.name,
      body: {
        'id': id,
        'clock_in_at': at.toIso8601String(),
        if (selfie != null) 'selfie_object_key': selfie.objectKey,
        if (fix != null) 'gps_lat': fix.lat,
        if (fix != null) 'gps_lng': fix.lng,
      },
      photos: [if (selfie != null) OutboxPhoto(id: selfie.id, kind: selfie.kind, path: selfie.path)],
    ));
    clock = ClockState(attendanceId: id, clockInAt: at);
    await db.put(CacheKeys.clock(_guardId), clock!.toJson());
    notifyListeners();
  }

  Future<void> clockOut({Fix? fix}) async {
    final at = _now;
    final attendanceId = clock?.attendanceId ?? session.guardStatus?.openAttendance?.id;
    await outbox.enqueue(OutboxOp(
      id: _uuid.v4(),
      kind: 'clock_out',
      method: 'POST',
      path: '/attendance/clock-out',
      guardId: _guardId,
      guardToken: _token,
      label: session.guard!.name,
      body: {
        if (attendanceId != null) 'attendance_id': attendanceId,
        'clock_out_at': at.toIso8601String(),
        if (fix != null) 'gps_lat': fix.lat,
        if (fix != null) 'gps_lng': fix.lng,
      },
    ));
    if (clock != null) {
      clock = ClockState(attendanceId: clock!.attendanceId, clockInAt: clock!.clockInAt, clockOutAt: at);
      await db.put(CacheKeys.clock(_guardId), clock!.toJson());
    }
    notifyListeners();
  }

  /// Whether the guard is clocked in (local record first, then the server).
  bool get clockedIn => clock != null ? clock!.open : session.guardStatus?.openAttendance != null;

  // ── patrol ─────────────────────────────────────────────────────────────────

  Future<void> startPatrol(PatrolRoute route) async {
    final sessionId = _uuid.v4();
    final at = _now;
    await outbox.enqueue(OutboxOp(
      id: _uuid.v4(),
      kind: 'session_start',
      method: 'POST',
      path: '/patrol/sessions',
      guardId: _guardId,
      guardToken: _token,
      label: route.name,
      body: {'id': sessionId, 'route_id': route.id, 'started_at': at.toIso8601String()},
    ));
    patrol = ActivePatrol(sessionId: sessionId, route: route, startedAt: at, scans: const []);
    await db.put(CacheKeys.patrol(_guardId), patrol!.toJson());
    session.patrolRunning = true;
    notifyListeners();
  }

  /// Records a checkpoint scan from a scanned QR [code] (signed `SP1:`
  /// payload or legacy text), or a manual check-in of [checkpoint]. Returns
  /// the checkpoint, or null when the code is not on the route. A repeated
  /// scan is ignored (returns the checkpoint).
  Future<Checkpoint?> scan(String code, {String method = 'qr', LocalPhoto? photo, Fix? fix, Checkpoint? checkpoint}) async {
    final p = patrol;
    if (p == null) return null;
    final match = checkpoint != null ? QrMatch(checkpoint) : p.route.matchQr(code);
    if (match == null) return null;
    final cp = match.checkpoint;
    if (p.scans.any((s) => s.checkpointId == cp.id)) return cp;
    final at = _now;
    await outbox.enqueue(OutboxOp(
      id: _uuid.v4(),
      kind: 'scan',
      method: 'POST',
      path: '/patrol/sessions/${p.sessionId}/scans',
      guardId: _guardId,
      guardToken: _token,
      label: cp.name,
      body: {
        'id': _uuid.v4(),
        // QR: the raw scanned text for the server to verify (signed `SP1:`
        // payload or legacy code, D-035); manual: the chosen checkpoint.
        if (checkpoint != null) 'checkpoint_id': cp.id else 'qr_payload': code.trim(),
        'method': method,
        'scanned_at': at.toIso8601String(),
        if (photo != null) 'photo_object_key': photo.objectKey,
        if (fix != null) 'gps_lat': fix.lat,
        if (fix != null) 'gps_lng': fix.lng,
      },
      photos: [if (photo != null) OutboxPhoto(id: photo.id, kind: photo.kind, path: photo.path)],
    ));
    patrol = ActivePatrol(sessionId: p.sessionId, route: p.route, startedAt: p.startedAt, scans: [...p.scans, LocalScan(checkpointId: cp.id, scannedAt: at, method: method)]);
    await db.put(CacheKeys.patrol(_guardId), patrol!.toJson());
    notifyListeners();
    return cp;
  }

  Future<void> endPatrol({bool abandoned = false}) async {
    final p = patrol;
    if (p == null) return;
    await outbox.enqueue(OutboxOp(
      id: _uuid.v4(),
      kind: 'session_end',
      method: 'POST',
      path: '/patrol/sessions/${p.sessionId}/end',
      guardId: _guardId,
      guardToken: _token,
      label: p.route.name,
      body: {'ended_at': _now.toIso8601String(), 'abandoned': abandoned},
    ));
    patrol = null;
    await db.put(CacheKeys.patrol(_guardId), null);
    session.patrolRunning = false;
    notifyListeners();
  }

  // ── reports ────────────────────────────────────────────────────────────────

  Future<void> reportIncident({required String type, required String severity, String? description, String? locationText, List<LocalPhoto> photos = const [], Fix? fix}) =>
      outbox.enqueue(OutboxOp(
        id: _uuid.v4(),
        kind: 'incident',
        method: 'POST',
        path: '/incidents',
        guardId: _guardId,
        guardToken: _token,
        label: type,
        body: {
          'id': _uuid.v4(),
          'type': type,
          'severity': severity,
          if (description != null && description.trim().isNotEmpty) 'description': description.trim(),
          if (locationText != null && locationText.trim().isNotEmpty) 'location': locationText.trim(),
          'photo_object_keys': photos.map((p) => p.objectKey).toList(),
          'occurred_at': _now.toIso8601String(),
          if (fix != null) 'gps_lat': fix.lat,
          if (fix != null) 'gps_lng': fix.lng,
        },
        photos: photos.map((p) => OutboxPhoto(id: p.id, kind: p.kind, path: p.path)).toList(),
      ));

  Future<void> reportDefect({required String type, String? description, String? locationText, LocalPhoto? photo}) => outbox.enqueue(OutboxOp(
        id: _uuid.v4(),
        kind: 'defect',
        method: 'POST',
        path: '/defects',
        guardId: _guardId,
        guardToken: _token,
        label: type,
        body: {
          'id': _uuid.v4(),
          'defect_type': type.trim(),
          if (description != null && description.trim().isNotEmpty) 'description': description.trim(),
          if (locationText != null && locationText.trim().isNotEmpty) 'location': locationText.trim(),
          if (photo != null) 'photo_object_key': photo.objectKey,
          'reported_at': _now.toIso8601String(),
        },
        photos: [if (photo != null) OutboxPhoto(id: photo.id, kind: photo.kind, path: photo.path)],
      ));

  Future<void> visitorIn({required String name, String? idType, String? idNumber, String? plate, String? host, String? purpose, LocalPhoto? photo}) async {
    final id = _uuid.v4();
    final at = _now;
    String? clean(String? s) => s == null || s.trim().isEmpty ? null : s.trim();
    await outbox.enqueue(OutboxOp(
      id: _uuid.v4(),
      kind: 'visitor',
      method: 'POST',
      path: '/visitors',
      guardId: _guardId,
      guardToken: _token,
      label: name,
      body: {
        'id': id,
        'visitor_name': name.trim(),
        if (clean(idType) != null) 'id_type': clean(idType),
        if (clean(idNumber) != null) 'id_number': clean(idNumber),
        if (clean(plate) != null) 'plate': clean(plate)!.toUpperCase(),
        if (clean(host) != null) 'host_name': clean(host),
        if (clean(purpose) != null) 'purpose': clean(purpose),
        if (photo != null) 'photo_object_key': photo.objectKey,
        'entry_at': at.toIso8601String(),
      },
      photos: [if (photo != null) OutboxPhoto(id: photo.id, kind: photo.kind, path: photo.path)],
    ));
    visitors = [LocalVisitor(id: id, name: name.trim(), entryAt: at, plate: clean(plate)?.toUpperCase(), host: clean(host), purpose: clean(purpose)), ...visitors];
    await db.put(CacheKeys.visitors, visitors.map((v) => v.toJson()).toList());
    notifyListeners();
  }

  Future<void> visitorOut(LocalVisitor v) async {
    final at = _now;
    await outbox.enqueue(OutboxOp(
      id: _uuid.v4(),
      kind: 'visitor_exit',
      method: 'POST',
      path: '/visitors/${v.id}/exit',
      guardId: _guardId,
      guardToken: _token,
      label: v.name,
      body: {'exit_at': at.toIso8601String()},
    ));
    visitors = [for (final x in visitors) x.id == v.id ? x.exited(at) : x];
    await db.put(CacheKeys.visitors, visitors.map((v) => v.toJson()).toList());
    notifyListeners();
  }

  // ── SOS ────────────────────────────────────────────────────────────────────

  /// Raises SOS: queued with top priority and retried every 10 s until the
  /// server confirms (FR-OFF-04). GPS is attached when available within 4 s.
  ///
  /// From the lock screen ([as] = the guard who tapped their name, MOB-3) no
  /// PIN is needed: the alert is sent without a guard token and the server
  /// flags it `guard_unverified` — an SOS is never blocked by a PIN.
  Future<void> sos({RosterGuard? as}) async {
    final id = _uuid.v4();
    final firedAt = _now;
    // Captured before the GPS wait: the tablet may lock meanwhile.
    final g = as ?? session.guard!;
    final guardId = g.id;
    final guardName = g.name;
    final token = as == null ? _token : null;
    _sosClear?.cancel();
    sosOutstanding.add(id);
    sosPreparing = true;
    notifyListeners();
    try {
      // A last known position is instant; a fresh fix may take seconds (or
      // wait on a permission dialog), so it must not delay the alarm.
      final quick = location.lastKnown();
      final op = OutboxOp(
        id: id,
        kind: 'sos',
        method: 'POST',
        path: '/sos',
        guardId: guardId,
        guardToken: token,
        priority: 0,
        label: guardName,
        body: {
          'id': id,
          'fired_at': firedAt.toIso8601String(),
          if (quick != null) 'gps_lat': quick.lat,
          if (quick != null) 'gps_lng': quick.lng,
        },
      );
      if (quick != null) {
        await outbox.enqueue(op);
      } else {
        // Persist first (survives a crash), then try to add a fix before the
        // first send goes out.
        await outbox.enqueue(op, send: false);
        final fix = await location.current(timeout: const Duration(seconds: 4));
        if (fix != null) await outbox.amendBody(id, {'gps_lat': fix.lat, 'gps_lng': fix.lng});
        unawaited(outbox.process());
      }
    } finally {
      sosPreparing = false;
      notifyListeners();
    }
  }

  /// Called when the outbox changes: once every SOS raised here has left the
  /// outbox (delivered), the green confirmation stays for 15 s, then hides.
  /// Waiting or rejected SOS keep the banner red (it reads the outbox itself).
  Future<void> refreshSosDelivery() async {
    if (sosOutstanding.isEmpty || sosPreparing) return;
    final queued = (await outbox.list()).where((o) => o.kind == 'sos').map((o) => o.id).toSet();
    if (sosOutstanding.any(queued.contains)) {
      _sosClear?.cancel();
      _sosClear = null;
    } else {
      _sosClear ??= Timer(const Duration(seconds: 15), dismissSosBanner);
    }
  }

  void dismissSosBanner() {
    _sosClear?.cancel();
    _sosClear = null;
    sosOutstanding.clear();
    notifyListeners();
  }

  @override
  void dispose() {
    _sosClear?.cancel();
    super.dispose();
  }

  Future<void> acknowledgeSos(SosAlert alert) async {
    await outbox.enqueue(OutboxOp(
      id: _uuid.v4(),
      kind: 'sos_ack',
      method: 'POST',
      path: '/sos/${alert.id}/acknowledge',
      guardId: _guardId,
      guardToken: _token,
      priority: 0,
      label: alert.guardName ?? '',
      body: const {},
    ));
    session.sosAlerts = session.sosAlerts.where((a) => a.id != alert.id).toList();
    unawaited(Future<void>.delayed(const Duration(seconds: 2), session.refreshSos));
    notifyListeners();
  }

  /// Forgets the guard's in-memory state when the tablet locks.
  void clear() {
    clock = null;
    patrol = null;
    notifyListeners();
  }

  @visibleForTesting
  String get siteIdForTest => _siteId;
}
