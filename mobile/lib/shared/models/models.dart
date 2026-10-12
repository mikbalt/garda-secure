// API models (see docs/api.md). Parsing is tolerant: unknown fields are
// ignored and optional fields may be missing. Timestamps arrive as RFC 3339
// UTC strings and are kept as UTC [DateTime]s; the tablet sits at the site, so
// the UI shows them in the device's local time.

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
  const Checkpoint({required this.id, required this.name, required this.qrCode, this.slaMinutes, this.orderIndex = 0});

  final String id;
  final String name;
  final String qrCode;
  final int? slaMinutes;
  final int orderIndex;

  factory Checkpoint.fromJson(Map<String, dynamic> j) => Checkpoint(
        id: j['id'] as String,
        name: (j['name'] as String?) ?? '',
        qrCode: (j['qr_code'] as String?) ?? '',
        slaMinutes: j['sla_minutes'] is num ? (j['sla_minutes'] as num).toInt() : null,
        orderIndex: _int(j['order_index']),
      );

  Map<String, dynamic> toJson() => {'id': id, 'name': name, 'qr_code': qrCode, 'sla_minutes': slaMinutes, 'order_index': orderIndex};
}

class PatrolRoute {
  const PatrolRoute({required this.id, required this.name, required this.checkpoints, this.active = true});

  final String id;
  final String name;
  final bool active;
  final List<Checkpoint> checkpoints;

  int get totalSlaMinutes => checkpoints.fold(0, (n, c) => n + (c.slaMinutes ?? 0));

  /// Finds a checkpoint by its scanned QR payload or ID.
  Checkpoint? byCode(String code) {
    final c = code.trim();
    for (final cp in checkpoints) {
      if (cp.qrCode == c || cp.id == c) return cp;
    }
    return null;
  }

  factory PatrolRoute.fromJson(Map<String, dynamic> j) {
    final cps = _list(j['checkpoints']).map(Checkpoint.fromJson).toList()..sort((a, b) => a.orderIndex.compareTo(b.orderIndex));
    return PatrolRoute(id: j['id'] as String, name: (j['name'] as String?) ?? '', active: j['active'] != false, checkpoints: cps);
  }

  Map<String, dynamic> toJson() => {'id': id, 'name': name, 'active': active, 'checkpoints': checkpoints.map((c) => c.toJson()).toList()};
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
  const GuardStatus({this.shift, this.shiftAttendance, this.openAttendance, this.sessionId, this.sessionRouteId, this.sessionStartedAt});

  final Shift? shift;
  final AttendanceRecord? shiftAttendance;
  final AttendanceRecord? openAttendance;
  final String? sessionId;
  final String? sessionRouteId;
  final DateTime? sessionStartedAt;

  factory GuardStatus.fromJson(Map<String, dynamic> j) {
    final sess = _map(_map(j['session'])['session']);
    return GuardStatus(
      shift: j['shift'] is Map ? Shift.fromJson(_map(j['shift'])) : null,
      shiftAttendance: j['shift_attendance'] is Map ? AttendanceRecord.fromJson(_map(j['shift_attendance'])) : null,
      openAttendance: j['open_attendance'] is Map ? AttendanceRecord.fromJson(_map(j['open_attendance'])) : null,
      sessionId: _str(sess['id']),
      sessionRouteId: _str(sess['route_id']),
      sessionStartedAt: _ts(sess['started_at']),
    );
  }
}
