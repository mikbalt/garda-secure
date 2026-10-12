import '../../shared/models/models.dart';

/// A scan recorded on this tablet (or rebuilt from the server).
class LocalScan {
  const LocalScan({required this.checkpointId, required this.scannedAt, this.method = 'qr'});

  final String checkpointId;
  final DateTime scannedAt;
  final String method;

  Map<String, dynamic> toJson() => {'checkpoint_id': checkpointId, 'scanned_at': scannedAt.toIso8601String(), 'method': method};

  factory LocalScan.fromJson(Map<String, dynamic> j) => LocalScan(
        checkpointId: j['checkpoint_id'] as String,
        scannedAt: DateTime.parse(j['scanned_at'] as String).toUtc(),
        method: (j['method'] as String?) ?? 'qr',
      );
}

enum CpStatus { onTime, late, overdue, missed, pending }

class CheckpointState {
  const CheckpointState({required this.checkpoint, required this.status, this.scannedAt, this.dueAt, this.method});

  final Checkpoint checkpoint;
  final CpStatus status;
  final DateTime? scannedAt;
  final DateTime? dueAt;
  final String? method;

  bool get scanned => scannedAt != null;
}

class PatrolProgress {
  const PatrolProgress({required this.items, required this.scanned, this.nextIndex});

  final List<CheckpointState> items;
  final int scanned;
  final int? nextIndex;

  int get total => items.length;
  double get ratio => total == 0 ? 0 : scanned / total;
  CheckpointState? get next => nextIndex == null ? null : items[nextIndex!];
}

/// Mirrors the server's SLA rule (backend service.ComputeProgress, D-008):
/// checkpoint i is due `sla_minutes` after the previous checkpoint's scan (the
/// session start for the first; the previous due time if it was skipped).
PatrolProgress computeProgress(PatrolRoute route, DateTime startedAt, List<LocalScan> scans, DateTime now, {bool ended = false}) {
  final first = <String, LocalScan>{};
  for (final s in scans) {
    final prev = first[s.checkpointId];
    if (prev == null || s.scannedAt.isBefore(prev.scannedAt)) first[s.checkpointId] = s;
  }
  var anchor = startedAt;
  var scanned = 0;
  int? next;
  final items = <CheckpointState>[];
  for (var i = 0; i < route.checkpoints.length; i++) {
    final cp = route.checkpoints[i];
    final due = cp.slaMinutes != null ? anchor.add(Duration(minutes: cp.slaMinutes!)) : null;
    final sc = first[cp.id];
    if (sc != null) {
      scanned++;
      items.add(CheckpointState(
        checkpoint: cp,
        status: due != null && sc.scannedAt.isAfter(due) ? CpStatus.late : CpStatus.onTime,
        scannedAt: sc.scannedAt,
        dueAt: due,
        method: sc.method,
      ));
      anchor = sc.scannedAt;
    } else {
      final status = ended
          ? CpStatus.missed
          : due != null && now.isAfter(due)
              ? CpStatus.overdue
              : CpStatus.pending;
      if (next == null && !ended) next = i;
      items.add(CheckpointState(checkpoint: cp, status: status, dueAt: due));
      if (due != null) anchor = due;
    }
  }
  return PatrolProgress(items: items, scanned: scanned, nextIndex: next);
}
