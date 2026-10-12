import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../../core/auth/auth_provider.dart';
import '../../l10n/strings.dart';
import '../../theme/app_theme.dart';
import 'setup_screen.dart';

/// Registered but not (yet) approved — or revoked. Polls every 10 s
/// (DeviceSession) and moves on by itself once an admin approves.
class PendingScreen extends StatelessWidget {
  const PendingScreen({super.key});

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final session = context.watch<DeviceSession>();
    final revoked = session.phase == Phase.revoked;
    return OnboardingFrame(
      step: 2,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Container(
            width: 72,
            height: 72,
            alignment: Alignment.center,
            decoration: BoxDecoration(color: (revoked ? AppColors.danger : AppColors.warn).withOpacity(0.12), shape: BoxShape.circle),
            child: Icon(revoked ? Icons.block_rounded : Icons.hourglass_top_rounded, size: 34, color: revoked ? AppColors.danger : AppColors.warn),
          ),
          const SizedBox(height: 24),
          Text(s.t(revoked ? 'pending.revoked' : 'pending.title'), style: Theme.of(context).textTheme.headlineMedium),
          const SizedBox(height: 10),
          Text(
            revoked ? s.t('pending.revokedBody') : s.t('pending.subtitle', {'name': session.device?.name ?? ''}),
            style: const TextStyle(fontSize: 15, color: AppColors.textMuted, height: 1.5),
          ),
          if (!revoked) ...[
            const SizedBox(height: 24),
            Row(children: [
              const SizedBox(width: 18, height: 18, child: CircularProgressIndicator(strokeWidth: 2.4)),
              const SizedBox(width: 12),
              Text(s.t('pending.checking'), style: const TextStyle(color: AppColors.textMuted)),
            ]),
          ],
          const SizedBox(height: 36),
          OutlinedButton.icon(
            onPressed: () => _confirmSignOut(context),
            icon: const Icon(Icons.logout),
            label: Text(s.t('pending.signOut')),
          ),
        ],
      ),
    );
  }

  static Future<void> _confirmSignOut(BuildContext context) async {
    final s = S.of(context);
    final ok = await showDialog<bool>(
      context: context,
      builder: (c) => AlertDialog(
        content: Text(s.t('dev.signOutConfirm')),
        actions: [
          TextButton(onPressed: () => Navigator.pop(c, false), child: Text(s.t('common.cancel'))),
          FilledButton(onPressed: () => Navigator.pop(c, true), child: Text(s.t('dev.signOut'))),
        ],
      ),
    );
    if (ok == true && context.mounted) await context.read<DeviceSession>().signOutDevice();
  }
}
