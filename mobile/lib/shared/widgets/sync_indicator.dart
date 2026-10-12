import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../../core/database/sync_service.dart';
import '../../features/sync/outbox_screen.dart';
import '../../l10n/strings.dart';
import '../../theme/app_theme.dart';

/// Compact online / queue indicator; opens the outbox.
class SyncIndicator extends StatelessWidget {
  const SyncIndicator({super.key, this.onDark = false});

  final bool onDark;

  @override
  Widget build(BuildContext context) {
    final o = context.watch<Outbox>();
    final s = S.of(context);
    final Color color;
    final IconData icon;
    final String text;
    if (o.failed > 0) {
      color = AppColors.danger;
      icon = Icons.error_outline;
      text = s.t('sync.failed', {'n': o.failed});
    } else if (o.pending > 0) {
      color = AppColors.warn;
      icon = o.online ? Icons.cloud_upload_outlined : Icons.cloud_off_outlined;
      text = s.t('sync.pending', {'n': o.pending});
    } else if (!o.online) {
      color = AppColors.textSubtle;
      icon = Icons.cloud_off_outlined;
      text = s.t('common.offline');
    } else {
      color = AppColors.ok;
      icon = Icons.cloud_done_outlined;
      text = s.t('common.online');
    }
    return Tooltip(
      message: s.t('sync.title'),
      child: InkWell(
        borderRadius: BorderRadius.circular(999),
        onTap: () => Navigator.of(context).push(MaterialPageRoute<void>(builder: (_) => const OutboxScreen())),
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
          decoration: BoxDecoration(
            color: onDark ? Colors.white.withOpacity(0.08) : color.withOpacity(0.08),
            borderRadius: BorderRadius.circular(999),
            border: Border.all(color: onDark ? Colors.white24 : color.withOpacity(0.3)),
          ),
          child: Row(mainAxisSize: MainAxisSize.min, children: [
            Icon(icon, size: 18, color: onDark ? Colors.white : color),
            const SizedBox(width: 6),
            Text(text, style: TextStyle(fontSize: 13, fontWeight: FontWeight.w600, color: onDark ? Colors.white : Color.lerp(color, Colors.black, 0.2))),
          ]),
        ),
      ),
    );
  }
}
