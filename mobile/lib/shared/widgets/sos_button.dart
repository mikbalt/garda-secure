import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:provider/provider.dart';

import '../../core/database/sync_service.dart';
import '../../features/field_actions.dart';
import '../../l10n/strings.dart';
import '../models/models.dart';
import '../../theme/app_theme.dart';

/// Panic button (FR-INC-02). Press and hold for 2 s — a ring fills while
/// holding — so it cannot fire from a stray tap, yet needs no extra dialog in
/// an emergency.
///
/// On the lock screen [asGuard] is the guard who tapped their name: the SOS
/// is raised for them without a PIN (MOB-3).
class SosButton extends StatefulWidget {
  const SosButton({super.key, this.compact = false, this.asGuard});

  final bool compact;
  final RosterGuard? asGuard;

  @override
  State<SosButton> createState() => _SosButtonState();
}

class _SosButtonState extends State<SosButton> with SingleTickerProviderStateMixin {
  late final AnimationController _hold = AnimationController(vsync: this, duration: const Duration(seconds: 2))
    ..addStatusListener((s) {
      if (s == AnimationStatus.completed) _fire();
    });

  Future<void> _fire() async {
    _hold.reset();
    await HapticFeedback.heavyImpact();
    if (!mounted) return;
    unawaited(context.read<FieldActions>().sos(as: widget.asGuard));
  }

  @override
  void dispose() {
    _hold.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final size = widget.compact ? 64.0 : 96.0;
    return Semantics(
      button: true,
      label: s.t('sos.hold'),
      onLongPress: _fire,
      child: GestureDetector(
        onTapDown: (_) => _hold.forward(from: 0),
        onTapUp: (_) => _hold.reverse(),
        onTapCancel: () => _hold.reverse(),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            SizedBox(
              width: size,
              height: size,
              child: AnimatedBuilder(
                animation: _hold,
                builder: (context, _) => Stack(
                  alignment: Alignment.center,
                  children: [
                    Container(
                      width: size,
                      height: size,
                      decoration: BoxDecoration(
                        shape: BoxShape.circle,
                        gradient: const LinearGradient(colors: [Color(0xFFF43F5E), Color(0xFFDC2626)], begin: Alignment.topLeft, end: Alignment.bottomRight),
                        boxShadow: [BoxShadow(color: AppColors.sos.withOpacity(0.35 + 0.3 * _hold.value), blurRadius: 18 + 18 * _hold.value, spreadRadius: 2)],
                      ),
                    ),
                    SizedBox(
                      width: size - 8,
                      height: size - 8,
                      child: CircularProgressIndicator(value: _hold.value, strokeWidth: 5, color: Colors.white, backgroundColor: Colors.white24),
                    ),
                    Text(s.t('sos.button'), style: TextStyle(color: Colors.white, fontWeight: FontWeight.w800, fontSize: size * 0.26, letterSpacing: 1)),
                  ],
                ),
              ),
            ),
            if (!widget.compact) ...[
              const SizedBox(height: 8),
              Text(s.t('sos.hold'), style: const TextStyle(fontSize: 12.5, color: AppColors.textMuted, fontWeight: FontWeight.w500)),
            ],
          ],
        ),
      ),
    );
  }
}

/// Banner while an SOS from this tablet has not reached the server yet
/// (FR-OFF-04: the guard must see that it is not confirmed).
class SosDeliveryBanner extends StatelessWidget {
  const SosDeliveryBanner({super.key});

  @override
  Widget build(BuildContext context) {
    final actions = context.watch<FieldActions>();
    final outbox = context.watch<Outbox>();
    // Read from the outbox (persisted), so a waiting or rejected SOS is still
    // shown after an app restart.
    final waiting = outbox.sosPending || actions.sosPreparing;
    final rejected = outbox.sosFailed;
    final delivered = !waiting && !rejected && actions.sosOutstanding.isNotEmpty;
    if (!waiting && !rejected && !delivered) return const SizedBox.shrink();
    final s = S.of(context);
    final String message;
    if (waiting && outbox.sosRejected) {
      message = s.t('sos.rejectedRetrying');
    } else if (waiting) {
      message = outbox.online ? s.t('sos.sending') : s.t('sos.notConfirmed');
    } else if (rejected) {
      message = s.t('sos.rejected');
    } else {
      message = s.t('sos.sent');
    }
    return Container(
      width: double.infinity,
      color: delivered ? AppColors.ok : AppColors.sos,
      padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 14),
      child: SafeArea(
        bottom: false,
        child: Row(
          children: [
            Icon(delivered ? Icons.check_circle : Icons.sos_rounded, color: Colors.white),
            const SizedBox(width: 12),
            Expanded(
              child: Text(
                message,
                style: const TextStyle(color: Colors.white, fontWeight: FontWeight.w600),
              ),
            ),
            if (delivered)
              IconButton(
                onPressed: actions.dismissSosBanner,
                icon: const Icon(Icons.close, color: Colors.white),
                tooltip: s.t('common.close'),
              ),
          ],
        ),
      ),
    );
  }
}
