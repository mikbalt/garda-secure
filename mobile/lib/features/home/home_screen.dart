import 'dart:async';

import 'package:flutter/material.dart';
import 'package:intl/intl.dart';
import 'package:provider/provider.dart';

import '../../core/auth/auth_provider.dart';
import '../../core/database/sync_service.dart';
import '../../core/services/clock.dart';
import '../../l10n/strings.dart';
import '../../shared/models/models.dart';
import '../../shared/widgets/sos_button.dart';
import '../../shared/widgets/sync_indicator.dart';
import '../../shared/widgets/ui.dart';
import '../../theme/app_theme.dart';
import '../attendance/clock_in_screen.dart';
import '../field_actions.dart';
import '../incident/defect_screen.dart';
import '../incident/incident_screen.dart';
import '../incident/visitors_screen.dart';
import '../patrol/patrol_screen.dart';
import '../patrol/progress.dart';
import '../sos/sos_alerts_screen.dart';
import '../sync/outbox_screen.dart';

/// `HH:mm` in the site's time zone (D-013, MOB-9).
String hm(DateTime t) => DateFormat.Hm().format(AppClock.site(t));

/// The unlocked guard's home: today's shift and clock status first, then a
/// running patrol, then the field actions. SOS is always one hold away.
class HomeScreen extends StatelessWidget {
  const HomeScreen({super.key});

  void _open(BuildContext context, Widget screen) => Navigator.of(context).push(MaterialPageRoute<void>(builder: (_) => screen));

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final session = context.watch<DeviceSession>();
    final actions = context.watch<FieldActions>();
    final guard = session.guard;
    if (guard == null) return const SizedBox.shrink();
    final wide = MediaQuery.sizeOf(context).width >= 900;

    final tiles = <Widget>[
      ActionTile(
        icon: Icons.route_rounded,
        color: AppColors.primary,
        title: actions.patrol != null ? s.t('home.patrolRunning') : s.t('home.patrol'),
        subtitle: actions.patrol != null ? actions.patrol!.route.name : s.t('home.patrolSub'),
        highlight: actions.patrol != null,
        onTap: () => _open(context, const PatrolScreen()),
      ),
      ActionTile(icon: Icons.report_gmailerrorred_rounded, color: AppColors.overdue, title: s.t('home.incident'), subtitle: s.t('home.incidentSub'), onTap: () => _open(context, const IncidentScreen())),
      ActionTile(icon: Icons.build_circle_outlined, color: AppColors.warn, title: s.t('home.defect'), subtitle: s.t('home.defectSub'), onTap: () => _open(context, const DefectScreen())),
      ActionTile(
        icon: Icons.badge_outlined,
        color: const Color(0xFF8B5CF6),
        title: s.t('home.visitors'),
        subtitle: s.t('home.visitorsSub'),
        badge: actions.visitors.where((v) => v.inside).isEmpty ? null : '${actions.visitors.where((v) => v.inside).length}',
        onTap: () => _open(context, const VisitorsScreen()),
      ),
    ];

    final failed = context.select<Outbox, int>((o) => o.failed);
    final due = actions.patrol == null ? nextDuePatrol(session.guardStatus?.patrolsDue ?? const []) : null;
    final main = ListView(
      padding: EdgeInsets.fromLTRB(wide ? 28 : 16, 20, wide ? 28 : 16, wide ? 28 : 120),
      children: [
        if (session.sosAlerts.isNotEmpty) ...[
          _SosAlertsCard(alerts: session.sosAlerts, canAck: guard.isSupervisor, onOpen: () => _open(context, const SosAlertsScreen())),
          const SizedBox(height: 16),
        ],
        if (failed > 0) ...[
          _Notice(
            icon: Icons.error_outline,
            color: AppColors.danger,
            title: s.t('sync.attention', {'n': failed}),
            body: s.t('sync.attentionSub'),
            onTap: () => _open(context, const OutboxScreen()),
          ),
          const SizedBox(height: 16),
        ],
        if (AppClock.skewed) ...[
          _Notice(
            icon: Icons.schedule_rounded,
            color: AppColors.warn,
            title: s.t('home.clockSkew', {'min': AppClock.offset.inMinutes.abs()}),
          ),
          const SizedBox(height: 16),
        ],
        const _ShiftCard(),
        if (due != null) ...[
          const SizedBox(height: 16),
          _DuePatrolCard(due: due, onTap: () => _open(context, const PatrolScreen())),
        ],
        if (actions.patrol != null) ...[
          const SizedBox(height: 16),
          _PatrolCard(patrol: actions.patrol!, onTap: () => _open(context, const PatrolScreen())),
        ],
        const SizedBox(height: 16),
        if (wide)
          GridView.count(
            shrinkWrap: true,
            physics: const NeverScrollableScrollPhysics(),
            crossAxisCount: 2,
            mainAxisSpacing: 14,
            crossAxisSpacing: 14,
            childAspectRatio: 3.6,
            children: tiles,
          )
        else
          ...[for (final t in tiles) Padding(padding: const EdgeInsets.only(bottom: 12), child: t)],
      ],
    );

    return Scaffold(
      appBar: AppBar(
        toolbarHeight: 72,
        titleSpacing: wide ? 28 : 16,
        title: Row(children: [
          Avatar(initials: guard.initials, size: 44),
          const SizedBox(width: 12),
          Expanded(
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Text(guard.name, style: const TextStyle(fontSize: 17, fontWeight: FontWeight.w600), overflow: TextOverflow.ellipsis),
              Row(children: [
                Flexible(child: _VerifiedBadge(verified: session.guardVerified)),
                const SizedBox(width: 8),
                Flexible(
                  child: Text(
                    [if (guard.isSupervisor) s.t('lock.supervisor'), session.site?.name ?? ''].where((e) => e.isNotEmpty).join(' · '),
                    style: const TextStyle(fontSize: 13, color: AppColors.textMuted, fontWeight: FontWeight.w400),
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
              ]),
            ]),
          ),
        ]),
        actions: [
          const SyncIndicator(),
          const SizedBox(width: 8),
          Padding(
            padding: EdgeInsets.only(right: wide ? 24 : 12),
            child: OutlinedButton.icon(
              style: OutlinedButton.styleFrom(minimumSize: const Size(0, 44)),
              onPressed: session.lock,
              icon: const Icon(Icons.lock_outline, size: 20),
              label: Text(s.t('home.lock')),
            ),
          ),
        ],
        bottom: const PreferredSize(preferredSize: Size.fromHeight(1), child: Divider()),
      ),
      body: wide
          ? Row(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              Expanded(child: main),
              Container(
                width: 260,
                decoration: const BoxDecoration(color: AppColors.surface, border: Border(left: BorderSide(color: AppColors.border))),
                child: Column(mainAxisAlignment: MainAxisAlignment.center, children: [
                  const SosButton(),
                  const SizedBox(height: 18),
                  Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 24),
                    child: Text(s.t('sos.confirmBody'), textAlign: TextAlign.center, style: Theme.of(context).textTheme.bodySmall),
                  ),
                ]),
              ),
            ])
          : main,
      floatingActionButtonLocation: FloatingActionButtonLocation.centerFloat,
      floatingActionButton: wide ? null : const SosButton(compact: true),
    );
  }
}

class _ShiftCard extends StatelessWidget {
  const _ShiftCard();

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final session = context.watch<DeviceSession>();
    final actions = context.watch<FieldActions>();
    final st = session.guardStatus;
    final shift = st?.shift;
    final now = AppClock.now();
    final clockedIn = actions.clockedIn;
    final inAt = actions.clock?.open == true ? actions.clock!.clockInAt : st?.openAttendance?.clockInAt;
    final doneShift = st?.shiftAttendance?.clockOutAt != null || (actions.clock != null && !actions.clock!.open);
    final outAt = actions.clock?.clockOutAt ?? st?.shiftAttendance?.clockOutAt;

    String shiftText;
    if (shift == null) {
      shiftText = s.t('home.noShift');
    } else if (shift.isRunning(now) || shift.start.difference(now).inHours < 12) {
      shiftText = s.t('home.shiftNow', {'start': hm(shift.start), 'end': hm(shift.end)});
    } else {
      shiftText = s.t('home.nextShift', {'day': DateFormat('EEE d MMM', s.lang).format(AppClock.site(shift.start)), 'start': hm(shift.start), 'end': hm(shift.end)});
    }

    final Color tone;
    final String statusText;
    if (clockedIn) {
      tone = AppColors.ok;
      statusText = s.t('home.clockedIn', {'time': inAt != null ? hm(inAt) : ''});
    } else if (doneShift && outAt != null) {
      tone = AppColors.textMuted;
      statusText = s.t('home.clockedOut', {'time': hm(outAt)});
    } else {
      tone = AppColors.warn;
      statusText = s.t('home.notClockedIn');
    }

    return SectionCard(
      padding: const EdgeInsets.all(22),
      child: Row(children: [
        IconTile(icon: Icons.schedule_rounded, color: tone, size: 56),
        const SizedBox(width: 18),
        Expanded(
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Text(shiftText, style: Theme.of(context).textTheme.titleLarge),
            const SizedBox(height: 6),
            Pill(text: statusText, color: tone, dot: true),
          ]),
        ),
        const SizedBox(width: 16),
        if (clockedIn)
          OutlinedButton.icon(
            onPressed: () => Navigator.of(context).push(MaterialPageRoute<void>(builder: (_) => const ClockScreen(clockOut: true))),
            icon: const Icon(Icons.logout_rounded),
            label: Text(s.t('home.clockOut')),
          )
        else
          FilledButton.icon(
            onPressed: () => Navigator.of(context).push(MaterialPageRoute<void>(builder: (_) => const ClockScreen(clockOut: false))),
            icon: const Icon(Icons.login_rounded),
            label: Text(s.t('home.clockIn')),
          ),
      ]),
    );
  }
}

class _PatrolCard extends StatefulWidget {
  const _PatrolCard({required this.patrol, required this.onTap});

  final ActivePatrol patrol;
  final VoidCallback onTap;

  @override
  State<_PatrolCard> createState() => _PatrolCardState();
}

class _PatrolCardState extends State<_PatrolCard> {
  late final Timer _t = Timer.periodic(const Duration(seconds: 30), (_) => setState(() {}));

  @override
  void dispose() {
    _t.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final p = widget.patrol.progress(AppClock.now());
    final next = p.next;
    final overdue = p.items.any((i) => i.status == CpStatus.overdue);
    return SectionCard(
      onTap: widget.onTap,
      borderColor: overdue ? AppColors.overdue : AppColors.primary100,
      color: overdue ? AppColors.overdueBg : AppColors.primary50,
      padding: const EdgeInsets.all(20),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(children: [
          const Icon(Icons.route_rounded, color: AppColors.primary),
          const SizedBox(width: 10),
          Expanded(child: Text(widget.patrol.route.name, style: Theme.of(context).textTheme.titleMedium)),
          Text(s.t('pat.progress', {'a': p.scanned, 'n': p.total}), style: const TextStyle(fontWeight: FontWeight.w600)),
          const Icon(Icons.chevron_right_rounded),
        ]),
        const SizedBox(height: 12),
        Row(children: [
          for (final i in p.items)
            Expanded(
              child: Container(
                height: 8,
                margin: const EdgeInsets.symmetric(horizontal: 2),
                decoration: BoxDecoration(
                  borderRadius: BorderRadius.circular(4),
                  color: switch (i.status) {
                    CpStatus.onTime => AppColors.ok,
                    CpStatus.late => AppColors.warn,
                    CpStatus.overdue => AppColors.overdue,
                    CpStatus.missed => AppColors.danger,
                    CpStatus.pending => const Color(0xFFCBD5E1),
                  },
                ),
              ),
            ),
        ]),
        if (next != null) ...[
          const SizedBox(height: 10),
          Text('${s.t('pat.next')}: ${next.checkpoint.name}', style: TextStyle(color: overdue ? const Color(0xFF9A3412) : AppColors.textMuted, fontWeight: FontWeight.w500)),
        ],
      ]),
    );
  }
}

/// Whether the acting guard's PIN was confirmed by the server (D-035).
class _VerifiedBadge extends StatelessWidget {
  const _VerifiedBadge({required this.verified});

  final bool verified;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    return Tooltip(
      message: s.t(verified ? 'sec.verifiedTip' : 'sec.unverifiedTip'),
      child: Pill(
        text: s.t(verified ? 'sec.verified' : 'sec.unverified'),
        color: verified ? AppColors.ok : AppColors.warn,
        icon: verified ? Icons.verified_user_outlined : Icons.cloud_off_outlined,
      ),
    );
  }
}

/// A warning row on top of the home screen.
class _Notice extends StatelessWidget {
  const _Notice({required this.icon, required this.color, required this.title, this.body, this.onTap});

  final IconData icon;
  final Color color;
  final String title;
  final String? body;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) => SectionCard(
        onTap: onTap,
        color: Color.alphaBlend(color.withOpacity(0.06), Colors.white),
        borderColor: color.withOpacity(0.35),
        child: Row(children: [
          IconTile(icon: icon, color: color, size: 44),
          const SizedBox(width: 14),
          Expanded(
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Text(title, style: const TextStyle(fontWeight: FontWeight.w600, height: 1.35)),
              if (body != null) ...[const SizedBox(height: 2), Text(body!, style: Theme.of(context).textTheme.bodySmall)],
            ]),
          ),
          if (onTap != null) const Icon(Icons.chevron_right_rounded, color: AppColors.textSubtle),
        ]),
      );
}

/// The next scheduled patrol that has not been started (D-035 schedules).
class _DuePatrolCard extends StatelessWidget {
  const _DuePatrolCard({required this.due, required this.onTap});

  final PatrolDue due;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final overdue = due.overdue(AppClock.now());
    final color = overdue ? AppColors.overdue : AppColors.primary;
    return SectionCard(
      onTap: onTap,
      color: overdue ? AppColors.overdueBg : AppColors.primary50,
      borderColor: overdue ? AppColors.overdue : AppColors.primary100,
      child: Row(children: [
        IconTile(icon: overdue ? Icons.alarm_rounded : Icons.event_repeat_rounded, color: color, size: 44),
        const SizedBox(width: 14),
        Expanded(
          child: Text(
            s.t(overdue ? 'home.patrolOverdue' : 'home.patrolDue', {'route': due.routeName, 'time': hm(due.dueAt)}),
            style: TextStyle(fontWeight: FontWeight.w600, color: overdue ? const Color(0xFF9A3412) : AppColors.text),
          ),
        ),
        const Icon(Icons.chevron_right_rounded, color: AppColors.textSubtle),
      ]),
    );
  }
}

class _SosAlertsCard extends StatelessWidget {
  const _SosAlertsCard({required this.alerts, required this.canAck, required this.onOpen});

  final List<SosAlert> alerts;
  final bool canAck;
  final VoidCallback onOpen;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final a = alerts.first;
    return SectionCard(
      color: AppColors.sos,
      borderColor: AppColors.sos,
      onTap: onOpen,
      child: Row(children: [
        const Icon(Icons.sos_rounded, color: Colors.white, size: 34),
        const SizedBox(width: 14),
        Expanded(
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Text(s.t('sos.alertBy', {'name': a.guardName ?? '—'}), style: const TextStyle(color: Colors.white, fontWeight: FontWeight.w700, fontSize: 16)),
            Text('${hm(a.firedAt)}${alerts.length > 1 ? ' · +${alerts.length - 1}' : ''}', style: const TextStyle(color: Colors.white70)),
          ]),
        ),
        FilledButton(
          style: FilledButton.styleFrom(backgroundColor: Colors.white, foregroundColor: AppColors.sos, minimumSize: const Size(0, 46)),
          onPressed: onOpen,
          child: Text(canAck ? s.t('sos.ack') : s.t('common.next')),
        ),
      ]),
    );
  }
}
