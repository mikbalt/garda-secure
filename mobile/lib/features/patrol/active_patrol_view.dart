import 'dart:async';

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../../core/auth/auth_provider.dart';
import '../../l10n/strings.dart';
import '../../shared/models/models.dart';
import '../../shared/widgets/sos_button.dart';
import '../../shared/widgets/sync_indicator.dart';
import '../../shared/widgets/ui.dart';
import '../../theme/app_theme.dart';
import '../field_actions.dart';
import 'manual_checkin_sheet.dart';
import 'patrol_widgets.dart';
import 'progress.dart';
import 'scanner_screen.dart';
import '../../core/services/clock.dart';

const _finishGreen = Color(0xFF059669);

/// The running patrol, tuned for a guard walking with the tablet: one big
/// "next checkpoint" card with the scan button, progress, and the route list.
class ActivePatrolView extends StatefulWidget {
  const ActivePatrolView({super.key, required this.patrol, required this.onEnd});

  final ActivePatrol patrol;

  /// Ends the patrol (finished or abandoned); the parent shows the outcome.
  final Future<void> Function({required bool abandoned}) onEnd;

  @override
  State<ActivePatrolView> createState() => _ActivePatrolViewState();
}

class _ActivePatrolViewState extends State<ActivePatrolView> {
  Timer? _tick;

  @override
  void initState() {
    super.initState();
    // Countdowns and overdue states move with the clock.
    _tick = Timer.periodic(const Duration(seconds: 15), (_) {
      if (mounted) setState(() {});
    });
  }

  @override
  void dispose() {
    _tick?.cancel();
    super.dispose();
  }

  void _touch() => context.read<DeviceSession>().touch();

  /// Yes/no dialog with large buttons; true when the guard confirms.
  Future<bool> _confirm({required Widget icon, required String title, required String body, required String no, required String yes, Color? yesColor}) async {
    final res = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        icon: icon,
        title: Text(title, textAlign: TextAlign.center),
        content: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 420),
          child: Text(body, textAlign: TextAlign.center, style: const TextStyle(fontSize: 16, height: 1.45, color: AppColors.textMuted)),
        ),
        actionsPadding: const EdgeInsets.fromLTRB(20, 4, 20, 20),
        actions: [
          Row(children: [
            Expanded(child: OutlinedButton(onPressed: () => Navigator.pop(ctx, false), child: Text(no, textAlign: TextAlign.center))),
            const SizedBox(width: 12),
            Expanded(
              child: FilledButton(
                style: yesColor == null ? null : FilledButton.styleFrom(backgroundColor: yesColor),
                onPressed: () => Navigator.pop(ctx, true),
                child: Text(yes, textAlign: TextAlign.center),
              ),
            ),
          ]),
        ],
      ),
    );
    return res == true;
  }

  Future<void> _scan() async {
    _touch();
    final res = await Navigator.of(context).push<ScanResult>(MaterialPageRoute(builder: (_) => const ScannerScreen(), fullscreenDialog: true));
    if (!mounted || res == null) return;
    if (res.manual) return _manual();
    await _recorded(res.checkpoint);
  }

  Future<void> _manual([Checkpoint? initial]) async {
    final cp = await showManualCheckIn(context, initial: initial);
    if (!mounted) return;
    await _recorded(cp);
  }

  Future<void> _recorded(Checkpoint? cp) async {
    if (cp == null) return;
    final s = S.of(context);
    showMessage(context, s.t('pat.scanned', {'name': cp.name}));
    final p = context.read<FieldActions>().patrol;
    if (p == null || p.progress(AppClock.now()).next != null) return;
    // Last checkpoint: offer to finish right away.
    final finish = await _confirm(
      icon: const Icon(Icons.task_alt_rounded, color: AppColors.ok, size: 44),
      title: s.t('pat2.allDone'),
      body: s.t('pat2.allDonePrompt'),
      no: s.t('pat2.later'),
      yes: s.t('pat.finish'),
      yesColor: _finishGreen,
    );
    if (finish && mounted) await widget.onEnd(abandoned: false);
  }

  Future<void> _finish(PatrolProgress progress) async {
    _touch();
    final left = progress.total - progress.scanned;
    if (left > 0) {
      final s = S.of(context);
      final ok = await _confirm(
        icon: const Icon(Icons.flag_rounded, color: AppColors.overdue, size: 40),
        title: s.t('pat2.finishTitle'),
        body: s.t('pat2.finishLeft', {'n': left}),
        no: s.t('pat2.keepGoing'),
        yes: s.t('pat2.finishYes'),
      );
      if (!ok || !mounted) return;
    }
    await widget.onEnd(abandoned: false);
  }

  Future<void> _abandon() async {
    _touch();
    final s = S.of(context);
    final ok = await _confirm(
      icon: const Icon(Icons.cancel_outlined, color: AppColors.danger, size: 40),
      title: s.t('pat2.abandonTitle'),
      body: s.t('pat2.abandonBody'),
      no: s.t('pat2.keepGoing'),
      yes: s.t('pat2.abandonYes'),
      yesColor: AppColors.danger,
    );
    if (ok && mounted) await widget.onEnd(abandoned: true);
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final now = AppClock.now();
    final patrol = widget.patrol;
    final progress = patrol.progress(now);
    final allDone = progress.next == null;

    final header = _ProgressHeader(patrol: patrol, progress: progress, now: now);
    final next = allDone
        ? _AllDoneCard(onFinish: () => _finish(progress))
        : _NextCard(progress: progress, now: now, onScan: _scan, onManual: () => _manual(progress.next!.checkpoint));

    return Scaffold(
      appBar: AppBar(
        title: Text(patrol.route.name, overflow: TextOverflow.ellipsis),
        actions: [
          const Center(child: SyncIndicator()),
          PopupMenuButton<String>(
            tooltip: '',
            icon: const Icon(Icons.more_vert_rounded),
            onOpened: _touch,
            onSelected: (v) {
              if (v == 'abandon') _abandon();
            },
            itemBuilder: (_) => [
              PopupMenuItem(
                value: 'abandon',
                height: 52,
                child: Row(children: [
                  const Icon(Icons.cancel_outlined, color: AppColors.danger),
                  const SizedBox(width: 12),
                  Text(s.t('pat.abandon'), style: const TextStyle(color: AppColors.danger, fontWeight: FontWeight.w600)),
                ]),
              ),
            ],
          ),
          const SizedBox(width: 4),
        ],
      ),
      body: LayoutBuilder(
        builder: (context, c) {
          if (c.maxWidth >= 900) {
            return Padding(
              padding: const EdgeInsets.fromLTRB(24, 20, 24, 20),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Expanded(
                    flex: 5,
                    child: ListView(children: [header, const SizedBox(height: 16), next]),
                  ),
                  const SizedBox(width: 20),
                  Expanded(flex: 6, child: _CheckpointList(progress: progress, now: now, scroll: true)),
                ],
              ),
            );
          }
          return ListView(
            padding: const EdgeInsets.fromLTRB(16, 16, 16, 24),
            children: [header, const SizedBox(height: 14), next, const SizedBox(height: 14), _CheckpointList(progress: progress, now: now, scroll: false)],
          );
        },
      ),
      bottomNavigationBar: _BottomBar(allDone: allDone, onFinish: () => _finish(progress)),
    );
  }
}

class _ProgressHeader extends StatelessWidget {
  const _ProgressHeader({required this.patrol, required this.progress, required this.now});

  final ActivePatrol patrol;
  final PatrolProgress progress;
  final DateTime now;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final tt = Theme.of(context).textTheme;
    return SectionCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.end,
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(s.t('pat2.progressLabel'), style: tt.bodySmall),
                    const SizedBox(height: 2),
                    Text(s.t('pat.progress', {'a': progress.scanned, 'n': progress.total}), style: tt.titleLarge),
                  ],
                ),
              ),
              Column(
                crossAxisAlignment: CrossAxisAlignment.end,
                children: [
                  Text(s.t('pat2.startedAt', {'time': hhmm(patrol.startedAt)}), style: tt.bodySmall),
                  const SizedBox(height: 2),
                  Row(mainAxisSize: MainAxisSize.min, children: [
                    const Icon(Icons.timer_outlined, size: 18, color: AppColors.textMuted),
                    const SizedBox(width: 4),
                    Text(s.t('pat2.elapsed', {'t': durationText(s, now.difference(patrol.startedAt))}), style: tt.titleMedium),
                  ]),
                ],
              ),
            ],
          ),
          const SizedBox(height: 14),
          SegmentedProgress(progress: progress),
        ],
      ),
    );
  }
}

class _NextCard extends StatelessWidget {
  const _NextCard({required this.progress, required this.now, required this.onScan, required this.onManual});

  final PatrolProgress progress;
  final DateTime now;
  final VoidCallback onScan;
  final VoidCallback onManual;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final st = progress.next!;
    final due = st.dueAt;
    final overdue = st.status == CpStatus.overdue;
    final left = due == null ? null : minutesLeft(due, now);
    final soon = !overdue && left != null && left <= 3;

    final Color dueColor = overdue ? AppColors.overdue : (soon ? const Color(0xFFB45309) : AppColors.textMuted);
    final String dueText = due == null
        ? s.t('pat2.noSla')
        : overdue
            ? s.t('pat.overdue', {'n': minutesOver(due, now)})
            : s.t('pat.dueIn', {'n': left});

    return SectionCard(
      padding: const EdgeInsets.all(22),
      color: overdue ? AppColors.overdueBg : null,
      borderColor: overdue ? AppColors.overdue.withOpacity(0.55) : AppColors.primary100,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              Pill(text: s.t('pat.next'), color: overdue ? AppColors.overdue : AppColors.primary, dot: true),
              const SizedBox(width: 10),
              Text(s.t('pat2.nextOf', {'i': progress.nextIndex! + 1, 'n': progress.total}), style: Theme.of(context).textTheme.bodySmall),
            ],
          ),
          const SizedBox(height: 14),
          Text(st.checkpoint.name, style: const TextStyle(fontSize: 30, fontWeight: FontWeight.w700, letterSpacing: -0.5, height: 1.15, color: AppColors.text)),
          const SizedBox(height: 10),
          Row(
            children: [
              Icon(overdue ? Icons.warning_amber_rounded : Icons.schedule_rounded, color: dueColor, size: 24),
              const SizedBox(width: 8),
              Flexible(child: Text(dueText, style: TextStyle(fontSize: 20, fontWeight: FontWeight.w700, color: dueColor))),
              if (due != null) ...[
                const SizedBox(width: 10),
                Text('· ${s.t('pat.due', {'time': hhmm(due)})}', style: const TextStyle(fontSize: 16, color: AppColors.textMuted)),
              ],
            ],
          ),
          const SizedBox(height: 22),
          SizedBox(
            height: 76,
            child: FilledButton.icon(
              onPressed: onScan,
              style: FilledButton.styleFrom(
                textStyle: const TextStyle(fontFamily: 'Inter', fontSize: 22, fontWeight: FontWeight.w700),
                shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
              ),
              icon: const Icon(Icons.qr_code_scanner_rounded, size: 32),
              label: Text(s.t('pat.scan')),
            ),
          ),
          const SizedBox(height: 12),
          SizedBox(
            height: 56,
            child: OutlinedButton.icon(
              onPressed: onManual,
              style: OutlinedButton.styleFrom(backgroundColor: AppColors.surface),
              icon: const Icon(Icons.photo_camera_outlined),
              label: Text(s.t('pat.manual')),
            ),
          ),
        ],
      ),
    );
  }
}

class _AllDoneCard extends StatelessWidget {
  const _AllDoneCard({required this.onFinish});

  final VoidCallback onFinish;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    return SectionCard(
      padding: const EdgeInsets.all(22),
      color: AppColors.okBg,
      borderColor: AppColors.ok.withOpacity(0.45),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const Icon(Icons.task_alt_rounded, color: AppColors.ok, size: 56),
          const SizedBox(height: 12),
          Text(s.t('pat2.allDone'), textAlign: TextAlign.center, style: const TextStyle(fontSize: 24, fontWeight: FontWeight.w700, color: AppColors.text)),
          const SizedBox(height: 6),
          Text(s.t('pat2.allDoneSub'), textAlign: TextAlign.center, style: Theme.of(context).textTheme.bodyMedium?.copyWith(color: AppColors.textMuted)),
          const SizedBox(height: 20),
          SizedBox(
            height: 64,
            child: FilledButton.icon(
              onPressed: onFinish,
              style: FilledButton.styleFrom(backgroundColor: _finishGreen, textStyle: const TextStyle(fontFamily: 'Inter', fontSize: 19, fontWeight: FontWeight.w700)),
              icon: const Icon(Icons.flag_rounded, size: 26),
              label: Text(s.t('pat.finish')),
            ),
          ),
        ],
      ),
    );
  }
}

class _CheckpointList extends StatelessWidget {
  const _CheckpointList({required this.progress, required this.now, required this.scroll});

  final PatrolProgress progress;
  final DateTime now;
  final bool scroll;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final rows = [
      for (var i = 0; i < progress.items.length; i++) _CheckpointRow(index: i, state: progress.items[i], next: i == progress.nextIndex, now: now),
    ];
    final title = Padding(
      padding: const EdgeInsets.fromLTRB(20, 16, 20, 14),
      child: Row(
        children: [
          Expanded(child: Text(s.t('pat2.checkpoints'), style: Theme.of(context).textTheme.titleMedium)),
          Text(s.t('pat.points', {'n': progress.total}), style: Theme.of(context).textTheme.bodySmall),
        ],
      ),
    );
    return SectionCard(
      padding: EdgeInsets.zero,
      child: Column(
        mainAxisSize: scroll ? MainAxisSize.max : MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          title,
          const Divider(),
          if (scroll)
            Expanded(child: ListView.separated(itemCount: rows.length, itemBuilder: (_, i) => rows[i], separatorBuilder: (_, __) => const Divider()))
          else
            for (var i = 0; i < rows.length; i++) ...[if (i > 0) const Divider(), rows[i]],
        ],
      ),
    );
  }
}

class _CheckpointRow extends StatelessWidget {
  const _CheckpointRow({required this.index, required this.state, required this.next, required this.now});

  final int index;
  final CheckpointState state;
  final bool next;
  final DateTime now;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final due = state.dueAt;
    final overdue = state.status == CpStatus.overdue;
    final parts = <String>[
      if (state.scannedAt != null) s.t('pat.scannedAt', {'time': hhmm(state.scannedAt!)}),
      if (due != null) s.t('pat.due', {'time': hhmm(due)}) else if (!state.scanned) s.t('pat2.noSla'),
      if (overdue && due != null) s.t('pat.overdue', {'n': minutesOver(due, now)}),
    ];
    final color = state.status == CpStatus.pending ? AppColors.textMuted : statusColor(state.status);
    final pills = [
      if (state.method == 'manual') Pill(text: s.t('pat2.manualBadge'), color: AppColors.textMuted, icon: Icons.photo_camera_outlined),
      Pill(text: statusLabel(s, state.status), color: color),
    ];
    return LayoutBuilder(builder: (context, c) {
      // Narrow screens put the status under the name so names don't wrap.
      final narrow = c.maxWidth < 480;
      return Container(
        color: next ? (overdue ? AppColors.overdueBg : AppColors.primary50) : null,
        padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 14),
        constraints: const BoxConstraints(minHeight: 72),
        child: Row(
          children: [
            CheckpointMarker(index: index, state: state, next: next),
            const SizedBox(width: 14),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(state.checkpoint.name, style: TextStyle(fontSize: 16.5, fontWeight: next ? FontWeight.w700 : FontWeight.w600, color: AppColors.text)),
                  const SizedBox(height: 3),
                  Text(
                    parts.join(' · '),
                    style: TextStyle(fontSize: 13.5, color: overdue ? AppColors.overdue : AppColors.textMuted, fontWeight: overdue ? FontWeight.w600 : FontWeight.w400),
                  ),
                  if (narrow) ...[
                    const SizedBox(height: 8),
                    Wrap(spacing: 6, runSpacing: 6, children: pills),
                  ],
                ],
              ),
            ),
            if (!narrow)
              for (final p in pills) ...[const SizedBox(width: 6), p],
          ],
        ),
      );
    });
  }
}

class _BottomBar extends StatelessWidget {
  const _BottomBar({required this.allDone, required this.onFinish});

  final bool allDone;
  final VoidCallback onFinish;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final finish = allDone
        ? FilledButton.icon(
            onPressed: onFinish,
            style: FilledButton.styleFrom(backgroundColor: _finishGreen),
            icon: const Icon(Icons.flag_rounded),
            label: Text(s.t('pat.finish')),
          )
        : OutlinedButton.icon(onPressed: onFinish, icon: const Icon(Icons.flag_outlined), label: Text(s.t('pat.finish')));
    return PatrolBottomBar(action: finish);
  }
}

/// Bottom bar of the patrol screens: SOS always within reach, plus an
/// optional main action on the right.
class PatrolBottomBar extends StatelessWidget {
  const PatrolBottomBar({super.key, this.action});

  final Widget? action;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final hint = Text(s.t('sos.hold'), style: const TextStyle(fontSize: 13.5, color: AppColors.textMuted, fontWeight: FontWeight.w500));
    return Material(
      color: AppColors.surface,
      shape: const Border(top: BorderSide(color: AppColors.border)),
      child: SafeArea(
        top: false,
        child: Padding(
          padding: const EdgeInsets.fromLTRB(16, 10, 20, 10),
          child: LayoutBuilder(
            builder: (context, c) {
              final wide = c.maxWidth >= 700;
              return Row(
                children: [
                  const SosButton(compact: true),
                  const SizedBox(width: 12),
                  if (action == null)
                    Expanded(child: hint)
                  else if (wide) ...[
                    hint,
                    const Spacer(),
                    SizedBox(width: 340, height: 56, child: action),
                  ] else
                    Expanded(child: SizedBox(height: 56, child: action)),
                ],
              );
            },
          ),
        ),
      ),
    );
  }
}
