import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../../core/auth/auth_provider.dart';
import '../../l10n/strings.dart';
import '../../shared/widgets/ui.dart';
import '../../theme/app_theme.dart';
import '../field_actions.dart';
import '../home/home_screen.dart' show hm;

/// Unacknowledged SOS alerts of the site (from any tablet). Supervisors can
/// acknowledge them here; other guards see who needs help and where.
class SosAlertsScreen extends StatelessWidget {
  const SosAlertsScreen({super.key});

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final session = context.watch<DeviceSession>();
    final canAck = session.guard?.isSupervisor ?? false;
    final alerts = session.sosAlerts;
    return Scaffold(
      appBar: AppBar(title: Text(s.t('sos.alertsTitle'))),
      body: RefreshIndicator(
        onRefresh: session.refreshSos,
        child: ListView(
          padding: const EdgeInsets.all(20),
          children: [
            if (alerts.isEmpty)
              Padding(
                padding: const EdgeInsets.only(top: 80),
                child: Column(children: [
                  const Icon(Icons.verified_user_outlined, size: 52, color: AppColors.ok),
                  const SizedBox(height: 12),
                  Text(s.t('sos.none'), style: const TextStyle(color: AppColors.textMuted, fontSize: 16)),
                ]),
              ),
            for (final a in alerts)
              Padding(
                padding: const EdgeInsets.only(bottom: 12),
                child: Constrained(
                  child: SectionCard(
                    borderColor: AppColors.sos.withOpacity(0.4),
                    padding: const EdgeInsets.all(20),
                    child: Row(children: [
                      const IconTile(icon: Icons.sos_rounded, color: AppColors.sos, size: 52),
                      const SizedBox(width: 16),
                      Expanded(
                        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                          Text(s.t('sos.alertBy', {'name': a.guardName ?? '—'}), style: Theme.of(context).textTheme.titleMedium),
                          const SizedBox(height: 4),
                          Text(
                            [hm(a.firedAt), if (a.gpsLat != null) '${a.gpsLat!.toStringAsFixed(5)}, ${a.gpsLng!.toStringAsFixed(5)}'].join(' · '),
                            style: const TextStyle(color: AppColors.textMuted),
                          ),
                        ]),
                      ),
                      if (canAck)
                        FilledButton(
                          style: FilledButton.styleFrom(backgroundColor: AppColors.sos),
                          onPressed: () async {
                            await context.read<FieldActions>().acknowledgeSos(a);
                            if (context.mounted) showMessage(context, s.t('sos.acked'));
                          },
                          child: Text(s.t('sos.ack')),
                        ),
                    ]),
                  ),
                ),
              ),
          ],
        ),
      ),
    );
  }
}
