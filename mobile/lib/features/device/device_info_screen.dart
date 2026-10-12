import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../../core/auth/auth_provider.dart';
import '../../core/database/sync_service.dart';
import '../../l10n/strings.dart';
import '../../shared/widgets/ui.dart';
import '../../theme/app_theme.dart';

/// Tablet details, language, and signing the device out (from the lock screen).
class DeviceInfoScreen extends StatelessWidget {
  const DeviceInfoScreen({super.key});

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final session = context.watch<DeviceSession>();
    final outbox = context.watch<Outbox>();
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
                row(s.t('sync.title'), outbox.pending + outbox.failed == 0 ? s.t('sync.allSent') : s.t('sync.pending', {'n': outbox.pending + outbox.failed})),
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
              onPressed: () async {
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
                if (ok == true && context.mounted) {
                  Navigator.of(context).pop();
                  await session.signOutDevice();
                }
              },
              icon: const Icon(Icons.logout),
              label: Text(s.t('dev.signOut')),
            ),
          ]),
        ),
      ]),
    );
  }
}
