class Guard {
  final String id;
  final String name;
  final String email;
  final List<String> roles;

  Guard({
    required this.id,
    required this.name,
    required this.email,
    required this.roles,
  });

  factory Guard.fromJson(Map<String, dynamic> json) => Guard(
        id: json['id'] as String,
        name: json['name'] as String,
        email: json['email'] as String,
        roles: List<String>.from(json['roles'] as List),
      );

  Map<String, dynamic> toJson() => {
        'id': id,
        'name': name,
        'email': email,
        'roles': roles,
      };
}

class Site {
  final String id;
  final String name;
  final String address;
  final double latitude;
  final double longitude;

  Site({
    required this.id,
    required this.name,
    required this.address,
    required this.latitude,
    required this.longitude,
  });

  factory Site.fromJson(Map<String, dynamic> json) => Site(
        id: json['id'] as String,
        name: json['name'] as String,
        address: json['address'] as String,
        latitude: (json['latitude'] as num).toDouble(),
        longitude: (json['longitude'] as num).toDouble(),
      );

  Map<String, dynamic> toJson() => {
        'id': id,
        'name': name,
        'address': address,
        'latitude': latitude,
        'longitude': longitude,
      };
}

class Shift {
  final String id;
  final String siteId;
  final String guardId;
  final DateTime startTime;
  final DateTime endTime;
  final String status;

  Shift({
    required this.id,
    required this.siteId,
    required this.guardId,
    required this.startTime,
    required this.endTime,
    required this.status,
  });

  factory Shift.fromJson(Map<String, dynamic> json) => Shift(
        id: json['id'] as String,
        siteId: json['site_id'] as String,
        guardId: json['guard_id'] as String,
        startTime: DateTime.parse(json['start_time'] as String),
        endTime: DateTime.parse(json['end_time'] as String),
        status: json['status'] as String,
      );

  Map<String, dynamic> toJson() => {
        'id': id,
        'site_id': siteId,
        'guard_id': guardId,
        'start_time': startTime.toIso8601String(),
        'end_time': endTime.toIso8601String(),
        'status': status,
      };
}

class AttendanceRecord {
  final String id;
  final String guardId;
  final String shiftId;
  final DateTime clockIn;
  final DateTime? clockOut;
  final double latitude;
  final double longitude;
  final bool synced;

  AttendanceRecord({
    required this.id,
    required this.guardId,
    required this.shiftId,
    required this.clockIn,
    this.clockOut,
    required this.latitude,
    required this.longitude,
    this.synced = false,
  });

  factory AttendanceRecord.fromJson(Map<String, dynamic> json) =>
      AttendanceRecord(
        id: json['id'] as String,
        guardId: json['guard_id'] as String,
        shiftId: json['shift_id'] as String,
        clockIn: DateTime.parse(json['clock_in'] as String),
        clockOut: json['clock_out'] != null
            ? DateTime.parse(json['clock_out'] as String)
            : null,
        latitude: (json['latitude'] as num).toDouble(),
        longitude: (json['longitude'] as num).toDouble(),
        synced: (json['synced'] as int? ?? 0) == 1,
      );

  Map<String, dynamic> toJson() => {
        'id': id,
        'guard_id': guardId,
        'shift_id': shiftId,
        'clock_in': clockIn.toIso8601String(),
        'clock_out': clockOut?.toIso8601String(),
        'latitude': latitude,
        'longitude': longitude,
        'synced': synced ? 1 : 0,
      };
}

class Route {
  final String id;
  final String siteId;
  final String name;
  final List<Checkpoint> checkpoints;

  Route({
    required this.id,
    required this.siteId,
    required this.name,
    required this.checkpoints,
  });

  factory Route.fromJson(Map<String, dynamic> json) => Route(
        id: json['id'] as String,
        siteId: json['site_id'] as String,
        name: json['name'] as String,
        checkpoints: (json['checkpoints'] as List)
            .map((c) => Checkpoint.fromJson(c as Map<String, dynamic>))
            .toList(),
      );

  Map<String, dynamic> toJson() => {
        'id': id,
        'site_id': siteId,
        'name': name,
        'checkpoints': checkpoints.map((c) => c.toJson()).toList(),
      };
}

class Checkpoint {
  final String id;
  final String routeId;
  final String name;
  final String qrCode;
  final double latitude;
  final double longitude;
  final int orderIndex;

  Checkpoint({
    required this.id,
    required this.routeId,
    required this.name,
    required this.qrCode,
    required this.latitude,
    required this.longitude,
    required this.orderIndex,
  });

  factory Checkpoint.fromJson(Map<String, dynamic> json) => Checkpoint(
        id: json['id'] as String,
        routeId: json['route_id'] as String,
        name: json['name'] as String,
        qrCode: json['qr_code'] as String,
        latitude: (json['latitude'] as num).toDouble(),
        longitude: (json['longitude'] as num).toDouble(),
        orderIndex: json['order_index'] as int,
      );

  Map<String, dynamic> toJson() => {
        'id': id,
        'route_id': routeId,
        'name': name,
        'qr_code': qrCode,
        'latitude': latitude,
        'longitude': longitude,
        'order_index': orderIndex,
      };
}

class Incident {
  final String id;
  final String guardId;
  final String siteId;
  final String type;
  final String description;
  final double latitude;
  final double longitude;
  final List<String> photoUrls;
  final DateTime reportedAt;
  final bool synced;

  Incident({
    required this.id,
    required this.guardId,
    required this.siteId,
    required this.type,
    required this.description,
    required this.latitude,
    required this.longitude,
    required this.photoUrls,
    required this.reportedAt,
    this.synced = false,
  });

  factory Incident.fromJson(Map<String, dynamic> json) => Incident(
        id: json['id'] as String,
        guardId: json['guard_id'] as String,
        siteId: json['site_id'] as String,
        type: json['type'] as String,
        description: json['description'] as String,
        latitude: (json['latitude'] as num).toDouble(),
        longitude: (json['longitude'] as num).toDouble(),
        photoUrls: List<String>.from(json['photo_urls'] as List? ?? []),
        reportedAt: DateTime.parse(json['reported_at'] as String),
        synced: (json['synced'] as int? ?? 0) == 1,
      );

  Map<String, dynamic> toJson() => {
        'id': id,
        'guard_id': guardId,
        'site_id': siteId,
        'type': type,
        'description': description,
        'latitude': latitude,
        'longitude': longitude,
        'photo_urls': photoUrls,
        'reported_at': reportedAt.toIso8601String(),
        'synced': synced ? 1 : 0,
      };
}

class SosAlert {
  final String id;
  final String guardId;
  final double latitude;
  final double longitude;
  final DateTime triggeredAt;
  final String status;

  SosAlert({
    required this.id,
    required this.guardId,
    required this.latitude,
    required this.longitude,
    required this.triggeredAt,
    required this.status,
  });

  factory SosAlert.fromJson(Map<String, dynamic> json) => SosAlert(
        id: json['id'] as String,
        guardId: json['guard_id'] as String,
        latitude: (json['latitude'] as num).toDouble(),
        longitude: (json['longitude'] as num).toDouble(),
        triggeredAt: DateTime.parse(json['triggered_at'] as String),
        status: json['status'] as String,
      );

  Map<String, dynamic> toJson() => {
        'id': id,
        'guard_id': guardId,
        'latitude': latitude,
        'longitude': longitude,
        'triggered_at': triggeredAt.toIso8601String(),
        'status': status,
      };
}
