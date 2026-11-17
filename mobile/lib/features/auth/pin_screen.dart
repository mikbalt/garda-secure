import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:intl/intl.dart';
import 'package:provider/provider.dart';

import '../../core/auth/auth_provider.dart';
import '../../core/services/clock.dart';
import '../../l10n/strings.dart';
import '../../shared/models/models.dart';
import '../../shared/widgets/brand.dart';
import '../../shared/widgets/sos_button.dart';
import '../../shared/widgets/sync_indicator.dart';
import '../../shared/widgets/ui.dart';
import '../../theme/app_theme.dart';
import '../device/device_info_screen.dart';

/// Lock screen of the shared tablet (D-001): pick your name, enter your PIN.
/// The PIN is checked locally against the synced bcrypt hash, so switching
/// guards works offline, and by the server when it is reachable (D-035).
/// After picking a name, SOS can be held without a PIN (MOB-3).
class PinScreen extends StatefulWidget {
  const PinScreen({super.key});

  @override
  State<PinScreen> createState() => _PinScreenState();
}

class _PinScreenState extends State<PinScreen> {
  RosterGuard? _selected;
  String _query = '';

  void _select(RosterGuard g) {
    final wide = MediaQuery.sizeOf(context).width >= 900;
    if (wide) {
      setState(() => _selected = g);
    } else {
      Navigator.of(context).push(MaterialPageRoute<void>(builder: (_) => Scaffold(backgroundColor: AppColors.ink, body: SafeArea(child: PinPanel(guard: g, onCancel: () => Navigator.pop(context))))));
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final session = context.watch<DeviceSession>();
    final wide = MediaQuery.sizeOf(context).width >= 900;
    final roster = session.roster.where((g) => _query.isEmpty || g.name.toLowerCase().contains(_query.toLowerCase())).toList();

    final picker = Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(s.t('lock.title'), style: const TextStyle(color: Colors.white, fontSize: 28, fontWeight: FontWeight.w600, letterSpacing: -0.4)),
        const SizedBox(height: 6),
        Text(s.t('lock.subtitle'), style: const TextStyle(color: Colors.white60, fontSize: 15)),
        const SizedBox(height: 20),
        if (session.roster.length > 8)
          Padding(
            padding: const EdgeInsets.only(bottom: 16),
            child: TextField(
              onChanged: (v) => setState(() => _query = v),
              style: const TextStyle(color: Colors.white),
              decoration: InputDecoration(
                hintText: s.t('lock.search'),
                hintStyle: const TextStyle(color: Colors.white38),
                prefixIcon: const Icon(Icons.search, color: Colors.white54),
                filled: true,
                fillColor: Colors.white.withValues(alpha: 0.06),
                enabledBorder: OutlineInputBorder(borderRadius: BorderRadius.circular(12), borderSide: BorderSide(color: Colors.white.withValues(alpha: 0.1))),
                focusedBorder: OutlineInputBorder(borderRadius: BorderRadius.circular(12), borderSide: const BorderSide(color: AppColors.primary400)),
              ),
            ),
          ),
        Expanded(
          child: session.roster.isEmpty
              ? _EmptyRoster(onSync: session.syncReference)
              : GridView.builder(
                  gridDelegate: SliverGridDelegateWithMaxCrossAxisExtent(maxCrossAxisExtent: wide ? 220 : 200, mainAxisExtent: 148, crossAxisSpacing: 14, mainAxisSpacing: 14),
                  itemCount: roster.length,
                  itemBuilder: (_, i) => _GuardCard(guard: roster[i], selected: roster[i].id == _selected?.id, onTap: () => _select(roster[i])),
                ),
        ),
      ],
    );

    return Scaffold(
      backgroundColor: AppColors.ink,
      body: BrandBackdrop(
        child: SafeArea(
          child: Padding(
            padding: EdgeInsets.all(wide ? 32 : 20),
            child: Column(
              children: [
                const _LockTopBar(),
                if (session.sosAlerts.isNotEmpty) ...[const SizedBox(height: 16), _SosStrip(count: session.sosAlerts.length)],
                if (session.lockNotice != null) ...[const SizedBox(height: 16), _LockNotice(notice: session.lockNotice!, onClose: session.clearLockNotice)],
                SizedBox(height: wide ? 28 : 20),
                Expanded(
                  child: wide
                      ? Row(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
                          Expanded(flex: 3, child: picker),
                          const SizedBox(width: 28),
                          SizedBox(
                            width: 380,
                            child: Container(
                              decoration: BoxDecoration(color: Colors.white.withValues(alpha: 0.04), borderRadius: BorderRadius.circular(24), border: Border.all(color: Colors.white.withValues(alpha: 0.08))),
                              child: _selected == null
                                  ? _PickHint()
                                  : PinPanel(key: ValueKey(_selected!.id), guard: _selected!, onCancel: () => setState(() => _selected = null)),
                            ),
                          ),
                        ])
                      : picker,
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _LockTopBar extends StatelessWidget {
  const _LockTopBar();

  @override
  Widget build(BuildContext context) {
    final session = context.watch<DeviceSession>();
    return Row(
      children: [
        const BrandMark(size: 40),
        const SizedBox(width: 14),
        Expanded(
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Text(session.site?.name ?? 'GARDA', style: const TextStyle(color: Colors.white, fontSize: 17, fontWeight: FontWeight.w600), overflow: TextOverflow.ellipsis),
            Text(session.device?.name ?? '', style: const TextStyle(color: Colors.white54, fontSize: 13), overflow: TextOverflow.ellipsis),
          ]),
        ),
        if (MediaQuery.sizeOf(context).width >= 600) ...[const _Clock(), const SizedBox(width: 16)],
        const SyncIndicator(onDark: true),
        const SizedBox(width: 4),
        IconButton(
          tooltip: S.of(context).t('dev.title'),
          onPressed: () => Navigator.of(context).push(MaterialPageRoute<void>(builder: (_) => const DeviceInfoScreen())),
          icon: const Icon(Icons.settings_outlined, color: Colors.white70),
        ),
      ],
    );
  }
}

class _Clock extends StatefulWidget {
  const _Clock();

  @override
  State<_Clock> createState() => _ClockState();
}

class _ClockState extends State<_Clock> {
  late final Timer _t = Timer.periodic(const Duration(seconds: 15), (_) => setState(() {}));

  @override
  void dispose() {
    _t.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final now = AppClock.siteNow();
    final lang = S.of(context).lang;
    return Column(crossAxisAlignment: CrossAxisAlignment.end, children: [
      Text(DateFormat.Hm().format(now), style: const TextStyle(color: Colors.white, fontSize: 22, fontWeight: FontWeight.w600, fontFeatures: [FontFeature.tabularFigures()])),
      Text(DateFormat('EEE, d MMM', lang).format(now), style: const TextStyle(color: Colors.white54, fontSize: 12)),
    ]);
  }
}

class _SosStrip extends StatelessWidget {
  const _SosStrip({required this.count});

  final int count;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      decoration: BoxDecoration(color: AppColors.sos, borderRadius: BorderRadius.circular(14)),
      child: Row(children: [
        const Icon(Icons.sos_rounded, color: Colors.white),
        const SizedBox(width: 10),
        Expanded(child: Text('${s.t('sos.alertsTitle')} · $count', style: const TextStyle(color: Colors.white, fontWeight: FontWeight.w600))),
        Text(s.t('lock.subtitle'), style: const TextStyle(color: Colors.white70, fontSize: 13)),
      ]),
    );
  }
}

/// The server rejected a PIN that the tablet had accepted offline.
class _LockNotice extends StatelessWidget {
  const _LockNotice({required this.notice, required this.onClose});

  final Map<String, String> notice;
  final VoidCallback onClose;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final detail = switch (notice['code']) {
      'notAllowed' => s.t('sec.notAllowed'),
      'pinNotSet' => s.t('sec.pinNotSet'),
      _ => null,
    };
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.fromLTRB(16, 8, 4, 8),
      decoration: BoxDecoration(color: const Color(0xFF92400E), borderRadius: BorderRadius.circular(14)),
      child: Row(children: [
        const Icon(Icons.gpp_maybe_outlined, color: Colors.white),
        const SizedBox(width: 10),
        Expanded(
          child: Text(
            [s.t('sec.lockedByServer', {'name': notice['name'] ?? ''}), if (detail != null) detail].join(' '),
            style: const TextStyle(color: Colors.white, fontWeight: FontWeight.w500),
          ),
        ),
        IconButton(onPressed: onClose, tooltip: s.t('common.close'), icon: const Icon(Icons.close, color: Colors.white70)),
      ]),
    );
  }
}

class _GuardCard extends StatelessWidget {
  const _GuardCard({required this.guard, required this.selected, required this.onTap});

  final RosterGuard guard;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    return Material(
      color: selected ? AppColors.primary.withValues(alpha: 0.25) : Colors.white.withValues(alpha: 0.05),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(18),
        side: BorderSide(color: selected ? AppColors.primary400 : Colors.white.withValues(alpha: 0.08), width: selected ? 2 : 1),
      ),
      child: InkWell(
        borderRadius: BorderRadius.circular(18),
        onTap: onTap,
        child: Opacity(
          opacity: guard.hasPin ? 1 : 0.55,
          child: Padding(
            padding: const EdgeInsets.all(14),
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                Avatar(initials: guard.initials, size: 54, color: guard.isSupervisor ? const Color(0xFF8B9CFF) : const Color(0xFF7DD3FC)),
                const SizedBox(height: 10),
                Flexible(
                  child: Text(guard.name, maxLines: 2, textAlign: TextAlign.center, overflow: TextOverflow.ellipsis, style: const TextStyle(color: Colors.white, fontWeight: FontWeight.w600, height: 1.2)),
                ),
                const SizedBox(height: 4),
                if (guard.isSupervisor)
                  Text(s.t('lock.supervisor'), style: const TextStyle(color: Color(0xFFA5B4FC), fontSize: 12, fontWeight: FontWeight.w600))
                else if (guard.employeeId != null)
                  Text(guard.employeeId!, style: const TextStyle(color: Colors.white38, fontSize: 12)),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _PickHint extends StatelessWidget {
  @override
  Widget build(BuildContext context) => Center(
        child: Padding(
          padding: const EdgeInsets.all(32),
          child: Column(mainAxisSize: MainAxisSize.min, children: [
            Icon(Icons.touch_app_outlined, size: 44, color: Colors.white.withValues(alpha: 0.3)),
            const SizedBox(height: 12),
            Text(S.of(context).t('lock.subtitle'), textAlign: TextAlign.center, style: const TextStyle(color: Colors.white54)),
          ]),
        ),
      );
}

class _EmptyRoster extends StatelessWidget {
  const _EmptyRoster({required this.onSync});

  final Future<void> Function() onSync;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    return Center(
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 420),
        child: Column(mainAxisSize: MainAxisSize.min, children: [
          Icon(Icons.group_off_outlined, size: 48, color: Colors.white.withValues(alpha: 0.35)),
          const SizedBox(height: 14),
          Text(s.t('lock.empty'), textAlign: TextAlign.center, style: const TextStyle(color: Colors.white70, fontSize: 15, height: 1.45)),
          const SizedBox(height: 18),
          OutlinedButton.icon(
            style: OutlinedButton.styleFrom(foregroundColor: Colors.white, side: const BorderSide(color: Colors.white24)),
            onPressed: onSync,
            icon: const Icon(Icons.refresh),
            label: Text(s.t('lock.syncRoster')),
          ),
        ]),
      ),
    );
  }
}

/// PIN entry for one guard.
class PinPanel extends StatefulWidget {
  const PinPanel({super.key, required this.guard, required this.onCancel});

  final RosterGuard guard;
  final VoidCallback onCancel;

  @override
  State<PinPanel> createState() => _PinPanelState();
}

class _PinPanelState extends State<PinPanel> {
  String _pin = '';
  bool _checking = false;
  String? _error;
  Timer? _tick;

  @override
  void dispose() {
    _tick?.cancel();
    super.dispose();
  }

  void _press(String d) {
    if (_checking || _pin.length >= 6 || _wait > 0) return;
    HapticFeedback.selectionClick();
    setState(() {
      _pin += d;
      _error = null;
    });
    if (_pin.length == 6) _submit();
  }

  void _back() {
    if (_pin.isEmpty || _checking) return;
    setState(() => _pin = _pin.substring(0, _pin.length - 1));
  }

  int get _wait => context.read<DeviceSession>().pinAttempts.waitSeconds(widget.guard.id);

  void _startCountdown() {
    _tick?.cancel();
    _tick = Timer.periodic(const Duration(seconds: 1), (t) {
      if (!mounted || _wait == 0) t.cancel();
      if (mounted) setState(() {});
    });
  }

  Future<void> _submit() async {
    final s = S.of(context);
    if (_pin.length < 4 || _checking) return;
    final session = context.read<DeviceSession>();
    final nav = Navigator.of(context);
    setState(() => _checking = true);
    final res = await session.unlockWithPin(widget.guard, _pin);
    if (!mounted) return;
    if (res.status == UnlockStatus.unlocked) {
      if (nav.canPop()) nav.pop();
      return;
    }
    await HapticFeedback.vibrate();
    if (!mounted) return;
    setState(() {
      _checking = false;
      _pin = '';
      _error = switch (res.status) {
        UnlockStatus.wrongPin => s.t('lock.wrongPin', {'n': res.attemptsLeft}),
        UnlockStatus.notAllowed => s.t('sec.notAllowed'),
        UnlockStatus.pinNotSet => s.t('sec.pinNotSet'),
        UnlockStatus.rosterStale => s.t('sec.rosterStale'),
        _ => null,
      };
    });
    if (res.status == UnlockStatus.locked) _startCountdown();
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final g = widget.guard;
    final wait = _wait;
    if (!g.hasPin) {
      return _PanelFrame(guard: g, onCancel: widget.onCancel, child: Text(s.t('lock.noPin'), textAlign: TextAlign.center, style: const TextStyle(color: Color(0xFFFCD34D), height: 1.45)));
    }
    final stale = context.select<DeviceSession, bool>((x) => x.rosterStale);
    return _PanelFrame(
      guard: g,
      onCancel: widget.onCancel,
      child: Column(mainAxisSize: MainAxisSize.min, children: [
        Text(s.t('lock.enterPin'), style: const TextStyle(color: Colors.white60)),
        const SizedBox(height: 14),
        Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            for (var i = 0; i < 6; i++)
              AnimatedContainer(
                duration: const Duration(milliseconds: 120),
                margin: const EdgeInsets.symmetric(horizontal: 6),
                width: 14,
                height: 14,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: i < _pin.length ? Colors.white : Colors.transparent,
                  border: Border.all(color: i < 4 ? Colors.white54 : Colors.white24, width: 1.5),
                ),
              ),
          ],
        ),
        ConstrainedBox(
          constraints: const BoxConstraints(minHeight: 44),
          child: Center(
            child: _checking
                ? Row(mainAxisSize: MainAxisSize.min, children: [
                    const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white70)),
                    const SizedBox(width: 10),
                    Text(s.t('lock.checking'), style: const TextStyle(color: Colors.white70)),
                  ])
                : Text(
                    wait > 0 ? s.t('lock.tooMany', {'s': wait}) : (_error ?? (stale ? s.t('sec.rosterStale') : '')),
                    textAlign: TextAlign.center,
                    style: const TextStyle(color: Color(0xFFFCA5A5), fontWeight: FontWeight.w500),
                  ),
          ),
        ),
        _Keypad(onDigit: _press, onBack: _back, onOk: _pin.length >= 4 ? _submit : null, enabled: wait == 0 && !_checking),
      ]),
    );
  }
}

class _PanelFrame extends StatelessWidget {
  const _PanelFrame({required this.guard, required this.onCancel, required this.child});

  final RosterGuard guard;
  final VoidCallback onCancel;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    return SingleChildScrollView(
      padding: const EdgeInsets.fromLTRB(24, 16, 24, 24),
      child: Column(children: [
        Row(crossAxisAlignment: CrossAxisAlignment.center, children: [
          TextButton.icon(
            style: TextButton.styleFrom(foregroundColor: Colors.white60),
            onPressed: onCancel,
            icon: const Icon(Icons.arrow_back, size: 18),
            label: Text(s.t('common.back')),
          ),
          const Spacer(),
          // MOB-3: an emergency never waits for a PIN — always in view.
          Flexible(
            flex: 3,
            child: Text(s.t('sec.sosLockHint'), textAlign: TextAlign.right, style: const TextStyle(color: Colors.white70, fontSize: 12.5, height: 1.3)),
          ),
          const SizedBox(width: 10),
          SosButton(compact: true, asGuard: guard),
        ]),
        const SizedBox(height: 4),
        Avatar(initials: guard.initials, size: 64, color: guard.isSupervisor ? const Color(0xFF8B9CFF) : const Color(0xFF7DD3FC)),
        const SizedBox(height: 12),
        Text(s.t('lock.hello', {'name': guard.name.split(' ').first}), style: const TextStyle(color: Colors.white, fontSize: 20, fontWeight: FontWeight.w600)),
        const SizedBox(height: 20),
        child,
      ]),
    );
  }
}

class _Keypad extends StatelessWidget {
  const _Keypad({required this.onDigit, required this.onBack, required this.onOk, required this.enabled});

  final ValueChanged<String> onDigit;
  final VoidCallback onBack;
  final VoidCallback? onOk;
  final bool enabled;

  @override
  Widget build(BuildContext context) {
    Widget key(Widget child, VoidCallback? onTap, {Color? bg}) => Padding(
          padding: const EdgeInsets.all(6),
          child: Material(
            color: bg ?? Colors.white.withValues(alpha: 0.07),
            shape: const CircleBorder(),
            child: InkWell(
              customBorder: const CircleBorder(),
              onTap: enabled ? onTap : null,
              child: SizedBox(width: 74, height: 74, child: Center(child: child)),
            ),
          ),
        );
    Text digit(String d) => Text(d, style: const TextStyle(color: Colors.white, fontSize: 28, fontWeight: FontWeight.w500));
    return Column(mainAxisSize: MainAxisSize.min, children: [
      for (final row in const [
        ['1', '2', '3'],
        ['4', '5', '6'],
        ['7', '8', '9'],
      ])
        Row(mainAxisSize: MainAxisSize.min, children: [for (final d in row) key(digit(d), () => onDigit(d))]),
      Row(mainAxisSize: MainAxisSize.min, children: [
        key(const Icon(Icons.backspace_outlined, color: Colors.white70), onBack, bg: Colors.transparent),
        key(digit('0'), () => onDigit('0')),
        key(Icon(Icons.arrow_forward_rounded, color: onOk == null ? Colors.white24 : Colors.white, size: 30), onOk, bg: onOk == null ? Colors.transparent : AppColors.primary),
      ]),
    ]);
  }
}
