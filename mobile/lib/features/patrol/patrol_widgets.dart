import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../../l10n/strings.dart';
import '../../theme/app_theme.dart';
import 'progress.dart';
import '../../core/services/clock.dart';

/// Wall-clock time `HH:mm` in the site's time zone (MOB-9).
String hhmm(DateTime t) {
  final l = AppClock.site(t);
  return '${l.hour.toString().padLeft(2, '0')}:${l.minute.toString().padLeft(2, '0')}';
}

/// "23 mnt" or "1 j 05 mnt".
String durationText(S s, Duration d) {
  final m = math.max(0, d.inMinutes);
  if (m < 60) return s.t('common.minutes', {'n': m});
  return s.t('pat2.hm', {'h': m ~/ 60, 'm': (m % 60).toString().padLeft(2, '0')});
}

/// Whole minutes left before [due] (rounded up, so "sisa 1 mnt" until it passes).
int minutesLeft(DateTime due, DateTime now) => math.max(0, (due.difference(now).inSeconds / 60).ceil());

/// Whole minutes past [due], at least 1.
int minutesOver(DateTime due, DateTime now) => math.max(1, now.difference(due).inMinutes);

Color statusColor(CpStatus st) => switch (st) {
      CpStatus.onTime => AppColors.ok,
      CpStatus.late => AppColors.warn,
      CpStatus.overdue => AppColors.overdue,
      CpStatus.missed => AppColors.danger,
      CpStatus.pending => AppColors.textSubtle,
    };

String statusLabel(S s, CpStatus st) => s.t(switch (st) {
      CpStatus.onTime => 'pat.st.on_time',
      CpStatus.late => 'pat.st.late',
      CpStatus.overdue => 'pat.st.overdue',
      CpStatus.missed => 'pat.st.missed',
      CpStatus.pending => 'pat.st.pending',
    });

/// One segment per checkpoint: green on time, amber late, orange overdue,
/// red missed, grey pending; the next checkpoint is brand-tinted.
class SegmentedProgress extends StatelessWidget {
  const SegmentedProgress({super.key, required this.progress, this.height = 12});

  final PatrolProgress progress;
  final double height;

  @override
  Widget build(BuildContext context) {
    final items = progress.items;
    if (items.isEmpty) return SizedBox(height: height);
    return Row(
      children: [
        for (var i = 0; i < items.length; i++) ...[
          if (i > 0) const SizedBox(width: 5),
          Expanded(
            child: Container(
              height: height,
              decoration: BoxDecoration(
                color: _color(items[i], i == progress.nextIndex),
                borderRadius: BorderRadius.circular(height / 2),
              ),
            ),
          ),
        ],
      ],
    );
  }

  Color _color(CheckpointState st, bool next) {
    if (st.status == CpStatus.pending) return next ? AppColors.primary400 : AppColors.border;
    return statusColor(st.status);
  }
}

/// Numbered circle (or check) in front of a checkpoint row.
class CheckpointMarker extends StatelessWidget {
  const CheckpointMarker({super.key, required this.index, required this.state, this.next = false, this.size = 38});

  final int index;
  final CheckpointState state;
  final bool next;
  final double size;

  @override
  Widget build(BuildContext context) {
    final scanned = state.scanned;
    final color = scanned ? statusColor(state.status) : (state.status == CpStatus.overdue ? AppColors.overdue : (next ? AppColors.primary : AppColors.textSubtle));
    final filled = scanned || next;
    return Container(
      width: size,
      height: size,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        color: filled ? color : AppColors.surface,
        border: Border.all(color: color, width: 2),
      ),
      child: scanned
          ? Icon(Icons.check_rounded, color: Colors.white, size: size * 0.6)
          : Text('${index + 1}', style: TextStyle(fontWeight: FontWeight.w700, fontSize: size * 0.4, color: filled ? Colors.white : color)),
    );
  }
}
