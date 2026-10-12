// API models (see docs/api.md). Parsing is tolerant: unknown fields are
// ignored and optional fields may be missing. Timestamps arrive as RFC 3339
// UTC strings and are kept as UTC [DateTime]s; the UI shows them in the
// site's time zone (AppClock.site, D-013).

DateTime? _ts(Object? v) => v is String && v.isNotEmpty ? DateTime.tryParse(v)?.toUtc() : null;
String? _str(Object? v) => v is String && v.isNotEmpty ? v : null;
int _int(Object? v, [int def = 0]) => v is num ? v.toInt() : def;
double? _dbl(Object? v) => v is num ? v.toDouble() : null;
Map<String, dynamic> _map(Object? v) => v is Map ? v.cast<String, dynamic>() : const {};
List<Map<String, dynamic>> _list(Object? v) => v is List ? v.whereType<Map<dynamic, dynamic>>().map((e) => e.cast<String, dynamic>()).toList() : const [];

class SiteConfig {
  const SiteConfig({
    this.clockWindowMinutes = 15,
    this.lateGraceMinutes = 5,
    this.earlyOutGraceMinutes = 5,
    this.absentAfterMinutes = 30,
    this.idleLockMinutes = 5,
    this.language = 'id',
  });

  final int clockWindowMinutes;
  final int lateGraceMinutes;
  final int earlyOutGraceMinutes;
  final int absentAfterMinutes;
  final int idleLockMinutes;
  final String language;

  factory SiteConfig.fromJson(Map<String, dynamic> j) => SiteConfig(
        clockWindowMinutes: _int(j['clock_window_minutes'], 15),
        lateGraceMinutes: _int(j['late_grace_minutes'], 5),
        earlyOutGraceMinutes: _int(j['early_out_grace_minutes'], 5),
        absentAfterMinutes: _int(j['absent_after_minutes'], 30),
        idleLockMinutes: _int(j['idle_lock_minutes'], 5),
        language: _str(j['language']) ?? 'id',
      );

  Map<String, dynamic> toJson() => {
        'clock_window_minutes': clockWindowMinutes,
        'late_grace_minutes': lateGraceMinutes,
        'early_out_grace_minutes': earlyOutGraceMinutes,
        'absent_after_minutes': absentAfterMinutes,
        'idle_lock_minutes': idleLockMinutes,
        'language': language,
      };
}

class Site {
  const Site({required this.id, required this.name, this.address, this.timezone = 'Asia/Jakarta'});

  final String id;
  final String name;
  final String? address;
  final String timezone;

  factory Site.fromJson(Map<String, dynamic> j) => Site(
        id: j['id'] as String,
        name: (j['name'] as String?) ?? '',
        address: _str(j['address']),
        timezone: _str(j['timezone']) ?? 'Asia/Jakarta',
      );

  Map<String, dynamic> toJson() => {'id': id, 'name': name, 'address': address, 'timezone': timezone};
}

enum DeviceStatusKind { pending, active, revoked }

class Device {
  const Device({required this.id, required this.name, required this.status, this.siteId});

  final String id;
  final String name;
  final DeviceStatusKind status;
  final String? siteId;

  bool get isActive => status == DeviceStatusKind.active && siteId != null;

  factory Device.fromJson(Map<String, dynamic> j) => Device(
        id: j['id'] as String,
        name: (j['name'] as String?) ?? '',
        siteId: _str(j['site_id']),
        status: switch (j['status']) {
          'active' => DeviceStatusKind.active,
          'revoked' => DeviceStatusKind.revoked,
          _ => DeviceStatusKind.pending,
        },
      );

  Map<String, dynamic> toJson() => {'id': id, 'name': name, 'status': status.name, 'site_id': siteId};
}

class RosterGuard {
  const RosterGuard({required this.id, required this.name, this.employeeId, this.role = 'guard', this.pinHash});

  final String id;
  final String name;
  final String? employeeId;
  final String role;
  final String? pinHash;

  bool get isSupervisor => role == 'supervisor';
  bool get hasPin => pinHash != null && pinHash!.isNotEmpty;

  String get initials {
    final parts = name.trim().split(RegExp(r'\s+')).where((p) => p.isNotEmpty).take(2);
    return parts.map((p) => p[0].toUpperCase()).join();
  }

  factory RosterGuard.fromJson(Map<String, dynamic> j) => RosterGuard(
        id: j['id'] as String,
        name: (j['name'] as String?) ?? '',
        employeeId: _str(j['employee_id']),
        role: _str(j['role']) ?? 'guard',
        pinHash: _str(j['pin_hash']),
      );

  Map<String, dynamic> toJson() => {'id': id, 'name': name, 'employee_id': employeeId, 'role': role, 'pin_hash': pinHash};
}

class Checkpoint {
  const Checkpoint({required this.id, required this.name, this.qrCode = '', this.qrSigned = false, this.slaMinutes, this.orderIndex = 0});

  final String id;
  final String name;

  /// Legacy plain QR text; empty for signed checkpoints (the server no longer
  /// ships their codes to tablets, D-035).
  final String qrCode;

  /// The checkpoint's QR is a signed `SP1:` payload.
  final bool qrSigned;
  final int? slaMinutes;
  final int orderIndex;

  factory Checkpoint.fromJson(Map<String, dynamic> j) => Checkpoint(
        id: j['id'] as String,
        name: (j['name'] as String?) ?? '',
        qrCode: (j['qr_code'] as String?) ?? '',
        qrSigned: j['qr_signed'] == true,
        slaMinutes: j['sla_minutes'] is num ? (j['sla_minutes'] as num).toInt() : null,
        orderIndex: _int(j['order_index']),
      );

  Map<String, dynamic> toJson() =>
      {'id': id, 'name': name, 'qr_code': qrCode.isEmpty ? null : qrCode, 'qr_signed': qrSigned, 'sla_minutes': slaMinutes, 'order_index': orderIndex};
}

/// A signed checkpoint QR payload `SP1:<checkpoint uuid>:<signature>`. The
/// tablet cannot verify the signature (the key stays on the server); it
/// matches the checkpoint locally and sends the raw payload for the server
/// to verify (it may flag the scan `qr_invalid`).
class SignedQr {
  const SignedQr(this.checkpointId, this.raw);

  final String checkpointId;
  final String raw;

  static final _re = RegExp(r'^SP1:([0-9a-f-]{36}):[A-Za-z0-9_-]+$');

  static SignedQr? parse(String scanned) {
    final raw = scanned.trim();
    final m = _re.firstMatch(raw);
    return m == null ? null : SignedQr(m.group(1)!, raw);
  }
}

/// The result of matching a scanned code against a route.
class QrMatch {
  const QrMatch(this.checkpoint, {this.signed});

  final Checkpoint checkpoint;

  /// Set for signed payloads; null for legacy codes. Either way the raw
  /// scanned text is sent as `qr_payload`.
  final SignedQr? signed;
}

class PatrolRoute {
  const PatrolRoute({required this.id, required this.name, required this.checkpoints, this.active = true});

  final String id;
  final String name;
  final bool active;
  final List<Checkpoint> checkpoints;

  int get totalSlaMinutes => checkpoints.fold(0, (n, c) => n + (c.slaMinutes ?? 0));

  /// Matches a scanned QR: a signed `SP1:` payload by checkpoint ID, a legacy
  /// code by exact text. Anything else (incl. a bare checkpoint ID) is no
  /// match.
  QrMatch? matchQr(String scanned) {
    final signed = SignedQr.parse(scanned);
    if (signed != null) {
      for (final cp in checkpoints) {
        if (cp.id.toLowerCase() == signed.checkpointId) return QrMatch(cp, signed: signed);
      }
      return null;
    }
    final c = scanned.trim();
    if (c.isEmpty) return null;
    for (final cp in checkpoints) {
      if (!cp.qrSigned && cp.qrCode.isNotEmpty && cp.qrCode == c) return QrMatch(cp);
    }
    return null;
  }

  /// Finds a checkpoint by its scanned QR payload.
  Checkpoint? byCode(String code) => matchQr(code)?.checkpoint;

  factory PatrolRoute.fromJson(Map<String, dynamic> j) {
    final cps = _list(j['checkpoints']).map(Checkpoint.fromJson).toList()..sort((a, b) => a.orderIndex.compareTo(b.orderIndex));
    return PatrolRoute(id: j['id'] as String, name: (j['name'] as String?) ?? '', active: j['active'] != false, checkpoints: cps);
  }

  Map<String, dynamic> toJson() => {'id': id, 'name': name, 'active': active, 'checkpoints': checkpoints.map((c) => c.toJson()).toList()};
}

/// A scheduled patrol of a route (`patrols_due` of `/device/status`).
class PatrolDue {
  const PatrolDue({required this.routeId, required this.routeName, required this.dueAt, this.graceUntil, this.status = 'pending'});

  final String routeId;
  final String routeName;
  final DateTime dueAt;
  final DateTime? graceUntil;

  /// pending | missed (past the grace time) — the server lists only patrols
  /// that were not started; `started` is tolerated and ignored.
  final String status;

  bool get pending => status == 'pending' || status == 'missed';

  /// Not started although due.
  bool overdue(DateTime now) => status == 'missed' || (pending && now.isAfter(dueAt));

  static PatrolDue? fromJson(Map<String, dynamic> j) {
    final due = _ts(j['due_at']);
    final route = _str(j['route_id']);
    if (due == null || route == null) return null;
    return PatrolDue(routeId: route, routeName: _str(j['route_name']) ?? '', dueAt: due, graceUntil: _ts(j['grace_until']), status: _str(j['status']) ?? 'pending');
  }
}

/// The most relevant not-started scheduled patrol: the earliest pending one.
PatrolDue? nextDuePatrol(List<PatrolDue> due, {String? routeId}) {
  final open = due.where((d) => d.pending && (routeId == null || d.routeId == routeId)).toList()..sort((a, b) => a.dueAt.compareTo(b.dueAt));
  return open.isEmpty ? null : open.first;
}

class Shift {
  const Shift({required this.id, required this.start, required this.end});

  final String id;
  final DateTime start;
  final DateTime end;

  bool isRunning(DateTime now) => !now.isBefore(start) && now.isBefore(end);

  factory Shift.fromJson(Map<String, dynamic> j) => Shift(id: j['id'] as String, start: _ts(j['start_time'])!, end: _ts(j['end_time'])!);

  Map<String, dynamic> toJson() => {'id': id, 'start_time': start.toIso8601String(), 'end_time': end.toIso8601String()};
}

class AttendanceRecord {
  const AttendanceRecord({
    required this.id,
    required this.status,
    this.shiftId,
    this.clockInAt,
    this.clockOutAt,
    this.lateMinutes = 0,
    this.earlyOutMinutes = 0,
  });

  final String id;
  final String? shiftId;
  final String status; // present | late | absent
  final DateTime? clockInAt;
  final DateTime? clockOutAt;
  final int lateMinutes;
  final int earlyOutMinutes;

  bool get isOpen => clockInAt != null && clockOutAt == null;

  factory AttendanceRecord.fromJson(Map<String, dynamic> j) => AttendanceRecord(
        id: j['id'] as String,
        shiftId: _str(j['shift_id']),
        status: _str(j['status']) ?? 'present',
        clockInAt: _ts(j['clock_in_at']),
        clockOutAt: _ts(j['clock_out_at']),
        lateMinutes: _int(j['late_minutes']),
        earlyOutMinutes: _int(j['early_out_minutes']),
      );

  Map<String, dynamic> toJson() => {
        'id': id,
        'shift_id': shiftId,
        'status': status,
        'clock_in_at': clockInAt?.toIso8601String(),
        'clock_out_at': clockOutAt?.toIso8601String(),
        'late_minutes': lateMinutes,
        'early_out_minutes': earlyOutMinutes,
      };
}

class SosAlert {
  const SosAlert({required this.id, required this.guardId, required this.firedAt, this.guardName, this.gpsLat, this.gpsLng, this.acknowledgedAt});

  final String id;
  final String guardId;
  final String? guardName;
  final DateTime firedAt;
  final double? gpsLat;
  final double? gpsLng;
  final DateTime? acknowledgedAt;

  factory SosAlert.fromJson(Map<String, dynamic> j) => SosAlert(
        id: j['id'] as String,
        guardId: (j['guard_id'] as String?) ?? '',
        guardName: _str(j['guard_name']),
        firedAt: _ts(j['fired_at']) ?? DateTime.now().toUtc(),
        gpsLat: _dbl(j['gps_lat']),
        gpsLng: _dbl(j['gps_lng']),
        acknowledgedAt: _ts(j['acknowledged_at']),
      );
}

/// `GET /device/status` — where the acting guard stands right now.
class GuardStatus {
  const GuardStatus({
    this.shift,
    this.shiftAttendance,
    this.openAttendance,
    this.sessionId,
    this.sessionRouteId,
    this.sessionStartedAt,
    this.serverTime,
    this.guardVerified,
    this.patrolsDue = const [],
  });

  final Shift? shift;
  final AttendanceRecord? shiftAttendance;
  final AttendanceRecord? openAttendance;
  final String? sessionId;
  final String? sessionRouteId;
  final DateTime? sessionStartedAt;
  final DateTime? serverTime;

  /// The server accepted the guard token of this request (null: not sent).
  final bool? guardVerified;

  /// Scheduled patrols around now (empty when the server does not send them).
  final List<PatrolDue> patrolsDue;

  factory GuardStatus.fromJson(Map<String, dynamic> j) {
    final sess = _map(_map(j['session'])['session']);
    return GuardStatus(
      shift: j['shift'] is Map ? Shift.fromJson(_map(j['shift'])) : null,
      shiftAttendance: j['shift_attendance'] is Map ? AttendanceRecord.fromJson(_map(j['shift_attendance'])) : null,
      openAttendance: j['open_attendance'] is Map ? AttendanceRecord.fromJson(_map(j['open_attendance'])) : null,
      sessionId: _str(sess['id']),
      sessionRouteId: _str(sess['route_id']),
      sessionStartedAt: _ts(sess['started_at']),
      serverTime: _ts(j['server_time']),
      guardVerified: j['guard_verified'] is bool ? j['guard_verified'] as bool : null,
      patrolsDue: _list(j['patrols_due']).map(PatrolDue.fromJson).whereType<PatrolDue>().toList(),
    );
  }
}
