import 'dart:async';

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../../core/auth/auth_provider.dart';
import '../../core/database/sync_service.dart';
import '../../core/services/location_service.dart';
import '../../core/services/photo_service.dart';
import '../../l10n/strings.dart';
import '../../shared/widgets/ui.dart';
import '../../theme/app_theme.dart';
import '../field_actions.dart';
import '../home/home_screen.dart' show hm;

/// Clock in (selfie + GPS, FR-ATT-03) or clock out. Saved to the outbox, so
/// it works offline with the real event time.
class ClockScreen extends StatefulWidget {
  const ClockScreen({super.key, required this.clockOut});

  final bool clockOut;

  @override
  State<ClockScreen> createState() => _ClockScreenState();
}

class _ClockScreenState extends State<ClockScreen> {
  LocalPhoto? _selfie;
  Fix? _fix;
  bool _locating = true;
  bool _busy = false;

  @override
  void initState() {
    super.initState();
    unawaited(_locate());
  }

  Future<void> _locate() async {
    final fix = await context.read<FieldActions>().location.current();
    if (mounted) {
      setState(() {
        _fix = fix;
        _locating = false;
      });
    }
  }

  Future<void> _takeSelfie() async {
    final actions = context.read<FieldActions>();
    final site = context.read<DeviceSession>().site!;
    final old = _selfie;
    final p = await actions.photos.take(kind: 'selfie', siteId: site.id, front: true);
    if (p == null || !mounted) return;
    if (old != null) await actions.photos.discard(old);
    setState(() => _selfie = p);
  }

  /// False when the server status says there is no shift in the window.
  bool _inWindow(DeviceSession session) {
    final st = session.guardStatus;
    if (st == null) return true; // unknown (offline): let the server decide
    final shift = st.shift;
    if (shift == null) return false;
    final now = DateTime.now().toUtc();
    final opens = shift.start.subtract(Duration(minutes: session.siteConfig.clockWindowMinutes));
    return !now.isBefore(opens) && now.isBefore(shift.end);
  }

  Future<void> _submit() async {
    final s = S.of(context);
    final actions = context.read<FieldActions>();
    final online = context.read<Outbox>().online;
    setState(() => _busy = true);
    if (widget.clockOut) {
      await actions.clockOut(fix: _fix);
    } else {
      await actions.clockIn(selfie: _selfie, fix: _fix);
    }
    if (!mounted) return;
    final msg = s.t(widget.clockOut ? 'att.savedOut' : 'att.savedIn');
    showMessage(context, online ? msg : '$msg ${s.t('att.queued')}');
    Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final session = context.watch<DeviceSession>();
    final shift = session.guardStatus?.shift;
    final out = widget.clockOut;
    final blocked = !out && !_inWindow(session);

    return Scaffold(
      appBar: AppBar(title: Text(s.t(out ? 'att.clockOutTitle' : 'att.clockInTitle'))),
      body: ListView(
        padding: const EdgeInsets.all(20),
        children: [
          Constrained(
            child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              SectionCard(
                padding: const EdgeInsets.all(22),
                child: Row(children: [
                  Avatar(initials: session.guard?.initials ?? '', size: 56),
                  const SizedBox(width: 16),
                  Expanded(
                    child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                      Text(session.guard?.name ?? '', style: Theme.of(context).textTheme.titleLarge),
                      const SizedBox(height: 4),
                      Text(
                        shift == null ? s.t('home.noShift') : s.t('home.shiftNow', {'start': hm(shift.start), 'end': hm(shift.end)}),
                        style: const TextStyle(color: AppColors.textMuted),
                      ),
                    ]),
                  ),
                  Column(crossAxisAlignment: CrossAxisAlignment.end, children: [
                    Text(hm(DateTime.now()), style: const TextStyle(fontSize: 30, fontWeight: FontWeight.w600, fontFeatures: [FontFeature.tabularFigures()])),
                    Text(session.site?.name ?? '', style: const TextStyle(fontSize: 13, color: AppColors.textMuted)),
                  ]),
                ]),
              ),
              if (blocked) ...[
                const SizedBox(height: 16),
                Container(
                  padding: const EdgeInsets.all(16),
                  decoration: BoxDecoration(color: AppColors.warnBg, borderRadius: BorderRadius.circular(14), border: Border.all(color: const Color(0xFFFDE68A))),
                  child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
                    const Icon(Icons.info_outline, color: AppColors.warn),
                    const SizedBox(width: 12),
                    Expanded(child: Text(s.t('att.noShift', {'n': session.siteConfig.clockWindowMinutes}), style: const TextStyle(color: Color(0xFF92400E), height: 1.45))),
                  ]),
                ),
              ],
              if (!out) ...[
                const SizedBox(height: 20),
                FieldLabel(s.t('att.selfie'), optional: s.t('common.optional')),
                Row(children: [
                  PhotoThumb(path: _selfie?.path, label: s.t('common.addPhoto'), size: 120, onTap: _takeSelfie),
                  const SizedBox(width: 16),
                  Expanded(child: Text(s.t('att.selfieHint'), style: const TextStyle(color: AppColors.textMuted, height: 1.45))),
                ]),
              ],
              const SizedBox(height: 20),
              FieldLabel(s.t('att.gps')),
              Row(children: [
                Icon(
                  _locating ? Icons.gps_not_fixed : (_fix != null ? Icons.gps_fixed : Icons.gps_off),
                  color: _locating ? AppColors.textMuted : (_fix != null ? AppColors.ok : AppColors.warn),
                ),
                const SizedBox(width: 10),
                Expanded(
                  child: Text(
                    _locating ? s.t('att.gpsWaiting') : (_fix != null ? s.t('att.gpsOk', {'m': _fix!.accuracy.round()}) : s.t('att.gpsNone')),
                    style: const TextStyle(color: AppColors.textMuted),
                  ),
                ),
              ]),
              const SizedBox(height: 32),
              FilledButton.icon(
                style: FilledButton.styleFrom(minimumSize: const Size.fromHeight(60), backgroundColor: out ? AppColors.ink : AppColors.primary),
                onPressed: _busy || blocked ? null : _submit,
                icon: Icon(out ? Icons.logout_rounded : Icons.login_rounded),
                label: Text(s.t(out ? 'att.confirmOut' : 'att.confirmIn')),
              ),
            ]),
          ),
        ],
      ),
    );
  }
}
