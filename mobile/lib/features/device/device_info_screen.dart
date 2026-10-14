import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../../core/api/tls.dart';
import '../../core/auth/auth_provider.dart';
import '../../core/database/sync_service.dart';
import '../../core/services/clock.dart';
import '../../core/services/health.dart';
import '../../core/services/push_service.dart';
import '../../l10n/strings.dart';
import '../../shared/widgets/ui.dart';
import '../../theme/app_theme.dart';
import '../auth/supervisor_gate.dart';
import '../sync/outbox_screen.dart';

/// Signs the device out (MOB-2): refused while items are queued, then a
/// supervisor PIN or the device password, then a confirmation. With
/// [gate] false (a never-approved tablet without roster) the approval step
/// is skipped — there is nothing on it to protect.
Future<void> protectedSignOut(BuildContext context, {bool gate = true}) async {
  final s = S.of(context);
  final session = context.read<DeviceSession>();
  final n = await session.db.outboxCount();
  if (!context.mounted) return;
  if (n > 0) {
    await showOutboxBlocked(context, s.t('dev.signOutBlocked', {'n': n}));
    return;
  }
  if (gate && !await requireSupervisor(context)) return;
  if (!context.mounted) return;
  final ok = await showDialog<bool>(
    context: context,
    builder: (c) => AlertDialog(
      content: Text(s.t('dev.signOutConfirm')),
      actions: [
        TextButton(onPressed: () => Navigator.pop(c, false), child: Text(s.t('common.cancel'))),
        FilledButton(style: FilledButton.styleFrom(backgroundColor: AppColors.danger), onPressed: () => Navigator.pop(c, true), child: Text(s.t('dev.signOut'))),
      ],
    ),
  );
  if (ok != true || !context.mounted) return;
  final nav = Navigator.of(context);
  try {
    await session.signOutDevice();
    if (nav.canPop()) nav.popUntil((r) => r.isFirst);
  } on OutboxNotEmpty catch (e) {
    if (context.mounted) await showOutboxBlocked(context, s.t('dev.signOutBlocked', {'n': e.count}));
  }
}

/// Explains that queued items block the action, with a way to the outbox.
Future<void> showOutboxBlocked(BuildContext context, String text) async {
  final s = S.of(context);
  final open = await showDialog<bool>(
    context: context,
    builder: (c) => AlertDialog(
      icon: const Icon(Icons.cloud_upload_outlined, color: AppColors.warn, size: 36),
      content: Text(text, style: const TextStyle(height: 1.45)),
      actions: [
        TextButton(onPressed: () => Navigator.pop(c, false), child: Text(s.t('common.close'))),
        FilledButton(onPressed: () => Navigator.pop(c, true), child: Text(s.t('dev.openOutbox'))),
      ],
    ),
  );
  if (open == true && context.mounted) {
    await Navigator.of(context).push(MaterialPageRoute<void>(builder: (_) => const OutboxScreen()));
  }
}

/// Push status (FCM) with a way to fix it: ask for the notification
/// permission again or resend the token.
class _PushRow extends StatelessWidget {
  const _PushRow({required this.push, required this.label});

  final PushService push;
  final String label;

  Future<void> _fix(BuildContext context) async {
    final s = S.of(context);
    if (push.state == PushState.denied) {
      final p = await push.requestPermission();
      if (p != PushPermission.granted && !await push.openSettings() && context.mounted) showMessage(context, s.t('push.deniedSettings'));
    } else {
      await push.register();
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final state = push.state;
    final (text, color) = switch (state) {
      PushState.active => (s.t('push.active'), AppColors.ok),
      PushState.unconfigured => (s.t('push.unconfigured'), AppColors.textMuted),
      PushState.denied => (s.t('push.denied'), AppColors.warn),
      PushState.tokenPending => (s.t('push.tokenPending'), AppColors.warn),
      PushState.error => (s.t('push.error'), AppColors.danger),
    };
    final action = switch (state) {
      PushState.denied => s.t('push.askAgain'),
      PushState.tokenPending || PushState.error => s.t('push.retry'),
      _ => null,
    };
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 10),
      child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
        SizedBox(width: 160, child: Text(label, style: const TextStyle(color: AppColors.textMuted))),
        Expanded(
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Text(text, style: TextStyle(fontWeight: FontWeight.w600, color: color)),
            const SizedBox(height: 2),
            Text(s.t(state == PushState.active ? 'push.hintActive' : 'push.hintFallback'), style: const TextStyle(color: AppColors.textMuted, fontSize: 13, height: 1.4)),
            if (action != null && push.canRetry) ...[
              const SizedBox(height: 6),
              TextButton.icon(
                style: TextButton.styleFrom(padding: EdgeInsets.zero, visualDensity: VisualDensity.compact),
                onPressed: () => _fix(context),
                icon: Icon(state == PushState.denied ? Icons.notifications_active_outlined : Icons.refresh, size: 18),
                label: Text(action),
              ),
            ],
          ]),
        ),
      ]),
    );
  }
}

/// Tablet details, language, and signing the device out (from the lock screen).
class DeviceInfoScreen extends StatelessWidget {
  const DeviceInfoScreen({super.key});

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final session = context.watch<DeviceSession>();
    final outbox = context.watch<Outbox>();
    final push = context.watch<PushService>();
    final offset = AppClock.offset;
    Widget row(String label, String value) => Padding(
          padding: const EdgeInsets.symmetric(vertical: 10),
          child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
            SizedBox(width: 160, child: Text(label, style: const TextStyle(color: AppColors.textMuted))),
            Expanded(child: Text(value, style: const TextStyle(fontWeight: FontWeight.w500))),
          ]),
        );
    return Scaffold(
      appBar: AppBar(title: Text(s.t('dev.title'))),
      body: ListView(padding: const EdgeInsets.all(20), children: [
        Constrained(
          child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
            SectionCard(
              child: Column(children: [
                row(s.t('dev.name'), session.device?.name ?? '—'),
                const Divider(),
                row(s.t('dev.site'), [session.site?.name, session.site?.address].whereType<String>().join('\n')),
                const Divider(),
                row(s.t('dev.server'), session.config?.serverUrl ?? '—'),
                const Divider(),
                row(s.t('dev.org'), session.config?.realm ?? '—'),
                const Divider(),
                row(s.t('dev.secureLink'), TrustStore.hasPrivateCa ? s.t('dev.privateCa') : s.t('dev.systemCa')),
                const Divider(),
                row(s.t('sync.title'), outbox.pending + outbox.failed == 0 ? s.t('sync.allSent') : s.t('sync.pending', {'n': outbox.pending + outbox.failed})),
                const Divider(),
                _PushRow(push: push, label: s.t('push.title')),
                const Divider(),
                row(s.t('dev.clockOffset'), '${offset.isNegative ? '−' : '+'}${(offset.inMilliseconds.abs() / 1000).toStringAsFixed(1)} s'),
                const Divider(),
                row(s.t('dev.version'), [AppInfo.version, if (AppInfo.build.isNotEmpty) '(${AppInfo.build})'].join(' ')),
              ]),
            ),
            const SizedBox(height: 20),
            FieldLabel(s.t('dev.language')),
            SegmentedButton<String>(
              segments: const [ButtonSegment(value: 'id', label: Text('Indonesia')), ButtonSegment(value: 'en', label: Text('English'))],
              selected: {session.locale.languageCode},
              showSelectedIcon: false,
              onSelectionChanged: (v) => session.setLanguage(v.first),
            ),
            const SizedBox(height: 32),
            OutlinedButton.icon(
              style: OutlinedButton.styleFrom(foregroundColor: AppColors.danger, side: const BorderSide(color: Color(0xFFFECACA))),
              onPressed: () => protectedSignOut(context),
              icon: const Icon(Icons.logout),
              label: Text(s.t('dev.signOut')),
            ),
          ]),
        ),
      ]),
    );
  }
}
