import 'dart:async';

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../../core/auth/auth_provider.dart';
import '../../l10n/strings.dart';
import '../../shared/models/models.dart';
import '../../shared/widgets/sync_indicator.dart';
import '../../shared/widgets/ui.dart';
import '../../theme/app_theme.dart';
import '../field_actions.dart';
import 'active_patrol_view.dart';
import 'progress.dart';
import 'patrol_widgets.dart';
import '../../core/services/clock.dart';

/// Patrol (FR-PAT-02..05), pushed full screen from home: the route picker
/// when no patrol runs, otherwise the running patrol, then a short summary.
class PatrolScreen extends StatelessWidget {
  const PatrolScreen({super.key});

  @override
  Widget build(BuildContext context) => const _PatrolFlow();
}

class _Summary {
  const _Summary({required this.route, required this.progress, required this.duration});

  final PatrolRoute route;
  final PatrolProgress progress;
  final Duration duration;
}

class _PatrolFlow extends StatefulWidget {
  const _PatrolFlow();

  @override
  State<_PatrolFlow> createState() => _PatrolFlowState();
}

class _PatrolFlowState extends State<_PatrolFlow> {
  _Summary? _summary;
  bool _leaving = false;

  Future<void> _end({required bool abandoned}) async {
    final actions = context.read<FieldActions>();
    final p = actions.patrol;
    if (p == null) return;
    final s = S.of(context);
    final now = AppClock.now();
    // Freeze the outcome before the local patrol is cleared.
    setState(() {
      if (abandoned) {
        _leaving = true;
      } else {
        _summary = _Summary(route: p.route, progress: computeProgress(p.route, p.startedAt, p.scans, now, ended: true), duration: now.difference(p.startedAt));
      }
    });
    await actions.endPatrol(abandoned: abandoned);
    if (!mounted || !abandoned) return;
    showMessage(context, s.t('pat2.abandoned'));
    await Navigator.of(context).maybePop();
  }

  @override
  Widget build(BuildContext context) {
    final summary = _summary;
    if (summary != null) return _CompletedView(summary: summary);
    final patrol = context.watch<FieldActions>().patrol;
    if (patrol != null) return ActivePatrolView(patrol: patrol, onEnd: _end);
    if (_leaving) return const Scaffold();
    return const _RoutePicker();
  }
}

// ── route picker ─────────────────────────────────────────────────────────────

class _RoutePicker extends StatefulWidget {
  const _RoutePicker();

  @override
  State<_RoutePicker> createState() => _RoutePickerState();
}

class _RoutePickerState extends State<_RoutePicker> {
  String? _starting;

  Future<void> _start(PatrolRoute route) async {
    if (_starting != null) return;
    context.read<DeviceSession>().touch();
    setState(() => _starting = route.id);
    try {
      await context.read<FieldActions>().startPatrol(route);
    } finally {
      if (mounted) setState(() => _starting = null);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final session = context.watch<DeviceSession>();
    final dueList = session.guardStatus?.patrolsDue ?? const <PatrolDue>[];
    final routes = session.routes.where((r) => r.active && r.checkpoints.isNotEmpty).toList();
    // Scheduled routes first, the most urgent on top (D-035 schedules).
    int dueRank(PatrolRoute r) => nextDuePatrol(dueList, routeId: r.id)?.dueAt.millisecondsSinceEpoch ?? 1 << 62;
    final order = {for (var i = 0; i < routes.length; i++) routes[i].id: i};
    routes.sort((a, b) {
      final c = dueRank(a).compareTo(dueRank(b));
      return c != 0 ? c : order[a.id]!.compareTo(order[b.id]!);
    });
    return Scaffold(
      appBar: AppBar(
        title: Text(s.t('pat2.title')),
        actions: const [Center(child: SyncIndicator()), SizedBox(width: 12)],
      ),
      bottomNavigationBar: const PatrolBottomBar(),
      body: LayoutBuilder(
        builder: (context, c) {
          final width = c.maxWidth.clamp(0.0, 1040.0) - 48;
          final cols = width >= 760 ? 2 : 1;
          final cardWidth = (width - (cols - 1) * 16) / cols;
          return ListView(
            padding: const EdgeInsets.fromLTRB(24, 24, 24, 32),
            children: [
              Constrained(
                maxWidth: 1040 - 48,
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    ScreenIntro(icon: Icons.route_rounded, color: AppColors.primary, title: s.t('pat.chooseRoute'), subtitle: s.t('pat.chooseRouteSub')),
                    const SizedBox(height: 24),
                    if (routes.isEmpty)
                      _EmptyRoutes(text: s.t('pat.noRoutes'))
                    else
                      Wrap(
                        spacing: 16,
                        runSpacing: 16,
                        children: [
                          for (final r in routes)
                            SizedBox(
                              width: cardWidth,
                              child: _RouteCard(route: r, due: nextDuePatrol(dueList, routeId: r.id), busy: _starting == r.id, disabled: _starting != null, onStart: () => _start(r)),
                            ),
                        ],
                      ),
                  ],
                ),
              ),
            ],
          );
        },
      ),
    );
  }
}

class _RouteCard extends StatelessWidget {
  const _RouteCard({required this.route, this.due, required this.busy, required this.disabled, required this.onStart});

  final PatrolRoute route;
  final PatrolDue? due;
  final bool busy;
  final bool disabled;
  final VoidCallback onStart;

  static const _preview = 3;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final tt = Theme.of(context).textTheme;
    final cps = route.checkpoints;
    final minutes = route.totalSlaMinutes;
    return SectionCard(
      padding: const EdgeInsets.all(20),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              const IconTile(icon: Icons.route_rounded, color: AppColors.primary, size: 48),
              const SizedBox(width: 14),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(route.name, style: tt.titleLarge),
                    const SizedBox(height: 6),
                    Wrap(spacing: 8, runSpacing: 6, children: [
                      Pill(text: s.t('pat.points', {'n': cps.length}), color: AppColors.primary, icon: Icons.place_outlined),
                      if (minutes > 0) Pill(text: s.t('pat.roundTime', {'n': minutes}), color: AppColors.textMuted, icon: Icons.schedule_rounded),
                      if (due != null)
                        due!.overdue(AppClock.now())
                            ? Pill(text: s.t('pat.overdueSince', {'time': hhmm(due!.dueAt)}), color: AppColors.overdue, icon: Icons.alarm_rounded)
                            : Pill(text: s.t('pat.dueAt', {'time': hhmm(due!.dueAt)}), color: AppColors.primary, icon: Icons.event_repeat_rounded),
                    ]),
                  ],
                ),
              ),
            ],
          ),
          const SizedBox(height: 16),
          for (var i = 0; i < cps.length && i < _preview; i++)
            Padding(
              padding: const EdgeInsets.only(bottom: 8),
              child: Row(
                children: [
                  Container(
                    width: 26,
                    height: 26,
                    alignment: Alignment.center,
                    decoration: BoxDecoration(shape: BoxShape.circle, border: Border.all(color: AppColors.border, width: 1.5)),
                    child: Text('${i + 1}', style: const TextStyle(fontSize: 12.5, fontWeight: FontWeight.w700, color: AppColors.textMuted)),
                  ),
                  const SizedBox(width: 12),
                  Expanded(child: Text(cps[i].name, style: const TextStyle(fontSize: 15, color: AppColors.text), overflow: TextOverflow.ellipsis)),
                  if (cps[i].slaMinutes != null) Text(s.t('common.minutes', {'n': cps[i].slaMinutes}), style: tt.bodySmall),
                ],
              ),
            ),
          if (cps.length > _preview)
            Padding(
              padding: const EdgeInsets.only(left: 38, bottom: 8),
              child: Text(s.t('pat2.more', {'n': cps.length - _preview}), style: tt.bodySmall),
            ),
          const SizedBox(height: 10),
          SizedBox(
            height: 60,
            child: FilledButton.icon(
              onPressed: disabled ? null : onStart,
              style: FilledButton.styleFrom(textStyle: const TextStyle(fontFamily: 'Inter', fontSize: 18, fontWeight: FontWeight.w700)),
              icon: busy
                  ? const SizedBox(width: 22, height: 22, child: CircularProgressIndicator(strokeWidth: 2.5, color: Colors.white))
                  : const Icon(Icons.play_arrow_rounded, size: 28),
              label: Text(s.t('pat.start')),
            ),
          ),
        ],
      ),
    );
  }
}

class _EmptyRoutes extends StatelessWidget {
  const _EmptyRoutes({required this.text});

  final String text;

  @override
  Widget build(BuildContext context) => SectionCard(
        padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 40),
        child: Column(
          children: [
            const IconTile(icon: Icons.wrong_location_outlined, color: AppColors.textMuted, size: 64),
            const SizedBox(height: 16),
            ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 440),
              child: Text(text, textAlign: TextAlign.center, style: Theme.of(context).textTheme.bodyMedium?.copyWith(color: AppColors.textMuted)),
            ),
          ],
        ),
      );
}

// ── finished ─────────────────────────────────────────────────────────────────

class _CompletedView extends StatefulWidget {
  const _CompletedView({required this.summary});

  final _Summary summary;

  @override
  State<_CompletedView> createState() => _CompletedViewState();
}

class _CompletedViewState extends State<_CompletedView> {
  static const _autoBack = 6;
  int _left = _autoBack;
  Timer? _timer;

  @override
  void initState() {
    super.initState();
    _timer = Timer.periodic(const Duration(seconds: 1), (t) {
      if (!mounted) return;
      if (_left <= 1) {
        t.cancel();
        _home();
      } else {
        setState(() => _left--);
      }
    });
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  void _home() {
    _timer?.cancel();
    Navigator.of(context).maybePop();
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final tt = Theme.of(context).textTheme;
    final items = widget.summary.progress.items;
    int count(CpStatus st) => items.where((i) => i.status == st).length;
    final onTime = count(CpStatus.onTime), late = count(CpStatus.late), missed = count(CpStatus.missed);
    return Scaffold(
      appBar: AppBar(automaticallyImplyLeading: false, title: Text(s.t('pat2.title'))),
      body: Center(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(24),
          child: Constrained(
            maxWidth: 520,
            child: SectionCard(
              padding: const EdgeInsets.fromLTRB(28, 32, 28, 24),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Center(
                    child: Container(
                      width: 96,
                      height: 96,
                      decoration: const BoxDecoration(color: AppColors.okBg, shape: BoxShape.circle),
                      child: const Icon(Icons.verified_rounded, color: AppColors.ok, size: 60),
                    ),
                  ),
                  const SizedBox(height: 18),
                  Text(s.t('pat.completed'), textAlign: TextAlign.center, style: tt.headlineMedium),
                  const SizedBox(height: 6),
                  Text(widget.summary.route.name, textAlign: TextAlign.center, style: tt.bodyMedium?.copyWith(color: AppColors.textMuted)),
                  const SizedBox(height: 18),
                  Wrap(
                    alignment: WrapAlignment.center,
                    spacing: 8,
                    runSpacing: 8,
                    children: [
                      Pill(text: s.t('pat2.sumOnTime', {'n': onTime}), color: AppColors.ok, dot: true),
                      if (late > 0) Pill(text: s.t('pat2.sumLate', {'n': late}), color: AppColors.warn, dot: true),
                      if (missed > 0) Pill(text: s.t('pat2.sumMissed', {'n': missed}), color: AppColors.danger, dot: true),
                      Pill(text: s.t('pat2.duration', {'t': durationText(s, widget.summary.duration)}), color: AppColors.textMuted, icon: Icons.timer_outlined),
                    ],
                  ),
                  const SizedBox(height: 26),
                  SizedBox(
                    height: 60,
                    child: FilledButton.icon(onPressed: _home, icon: const Icon(Icons.home_rounded), label: Text(s.t('pat2.backHome'))),
                  ),
                  const SizedBox(height: 10),
                  Text(s.t('pat2.autoBack', {'n': _left}), textAlign: TextAlign.center, style: tt.bodySmall),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
