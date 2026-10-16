import 'dart:async';

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../../core/auth/auth_provider.dart';
import '../../core/services/location_service.dart';
import '../../core/services/photo_service.dart';
import '../../l10n/strings.dart';
import '../../shared/models/models.dart';
import '../../shared/widgets/ui.dart';
import '../../theme/app_theme.dart';
import '../field_actions.dart';
import 'patrol_widgets.dart';
import 'progress.dart';
import '../../core/services/clock.dart';

/// Manual check-in (FR-PAT-03) when a QR code is damaged: pick the checkpoint
/// and take a required photo. Returns the recorded checkpoint, or null.
Future<Checkpoint?> showManualCheckIn(BuildContext context, {Checkpoint? initial}) {
  context.read<DeviceSession>().touch();
  return showModalBottomSheet<Checkpoint>(
    context: context,
    isScrollControlled: true,
    useSafeArea: true,
    showDragHandle: true,
    backgroundColor: AppColors.surface,
    constraints: const BoxConstraints(maxWidth: 680),
    shape: const RoundedRectangleBorder(borderRadius: BorderRadius.vertical(top: Radius.circular(24))),
    builder: (_) => ManualCheckInSheet(initial: initial),
  );
}

class ManualCheckInSheet extends StatefulWidget {
  const ManualCheckInSheet({super.key, this.initial});

  final Checkpoint? initial;

  @override
  State<ManualCheckInSheet> createState() => _ManualCheckInSheetState();
}

class _ManualCheckInSheetState extends State<ManualCheckInSheet> {
  late final FieldActions _actions = context.read<FieldActions>();
  Checkpoint? _cp;
  LocalPhoto? _photo;
  bool _busy = false;
  bool _saved = false;
  late final Future<Fix?> _fix;

  @override
  void initState() {
    super.initState();
    _cp = widget.initial ?? _actions.patrol?.progress(AppClock.now()).next?.checkpoint;
    // Started now so a fix is usually ready by the time the photo is taken.
    _fix = _actions.location.current(timeout: const Duration(seconds: 8)).catchError((Object _) => null);
  }

  @override
  void dispose() {
    final p = _photo;
    if (!_saved && p != null) unawaited(_actions.photos.discard(p));
    super.dispose();
  }

  Future<void> _takePhoto() async {
    final session = context.read<DeviceSession>()..touch();
    final s = S.of(context);
    LocalPhoto? shot;
    try {
      shot = await _actions.photos.take(kind: 'checkpoint', siteId: session.site!.id);
    } catch (_) {
      if (mounted) showMessage(context, s.t('pat2.photoFailed'), error: true);
      return;
    }
    if (shot == null || !mounted) return;
    final old = _photo;
    if (old != null) unawaited(_actions.photos.discard(old));
    setState(() => _photo = shot);
  }

  void _removePhoto() {
    final p = _photo;
    if (p == null) return;
    unawaited(_actions.photos.discard(p));
    setState(() => _photo = null);
  }

  Future<void> _submit() async {
    final cp = _cp;
    final photo = _photo;
    if (cp == null || photo == null || _busy) return;
    context.read<DeviceSession>().touch();
    setState(() => _busy = true);
    final fix = await _fix.timeout(const Duration(seconds: 2), onTimeout: () => null);
    final res = await _actions.scan(cp.qrCode, method: 'manual', photo: photo, fix: fix, checkpoint: cp);
    _saved = true;
    if (mounted) Navigator.of(context).pop(res);
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final tt = Theme.of(context).textTheme;
    final patrol = context.watch<FieldActions>().patrol;
    final now = AppClock.now();
    final progress = patrol?.progress(now);
    final open = <(int, CheckpointState)>[
      if (progress != null)
        for (var i = 0; i < progress.items.length; i++)
          if (!progress.items[i].scanned) (i, progress.items[i]),
    ];

    return Padding(
      padding: EdgeInsets.only(bottom: MediaQuery.viewInsetsOf(context).bottom),
      child: SingleChildScrollView(
        padding: const EdgeInsets.fromLTRB(24, 0, 24, 24),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(s.t('pat.manualTitle'), style: tt.headlineMedium),
            const SizedBox(height: 12),
            Container(
              padding: const EdgeInsets.all(14),
              decoration: BoxDecoration(color: AppColors.warnBg, borderRadius: BorderRadius.circular(12), border: Border.all(color: AppColors.warn.withValues(alpha: 0.35))),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Icon(Icons.info_outline_rounded, color: Color(0xFFB45309)),
                  const SizedBox(width: 10),
                  Expanded(child: Text(s.t('pat.manualHint'), style: const TextStyle(fontSize: 14.5, color: Color(0xFF78350F), height: 1.4))),
                ],
              ),
            ),
            const SizedBox(height: 22),
            FieldLabel(s.t('pat2.pickCheckpoint')),
            for (final (i, st) in open) ...[
              _CheckpointOption(
                index: i,
                state: st,
                now: now,
                selected: _cp?.id == st.checkpoint.id,
                onTap: _busy
                    ? null
                    : () {
                        context.read<DeviceSession>().touch();
                        setState(() => _cp = st.checkpoint);
                      },
              ),
              const SizedBox(height: 8),
            ],
            const SizedBox(height: 14),
            FieldLabel(s.t('pat2.photoLabel')),
            Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                PhotoThumb(path: _photo?.path, size: 128, label: s.t('common.addPhoto'), onTap: _busy ? null : _takePhoto, onRemove: _busy ? null : _removePhoto),
                const SizedBox(width: 16),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(s.t('pat2.photoHint'), style: tt.bodySmall?.copyWith(fontSize: 14)),
                      if (_photo != null) ...[
                        const SizedBox(height: 10),
                        TextButton.icon(onPressed: _busy ? null : _takePhoto, icon: const Icon(Icons.refresh_rounded), label: Text(s.t('common.retake'))),
                      ],
                    ],
                  ),
                ),
              ],
            ),
            const SizedBox(height: 28),
            Row(
              children: [
                Expanded(child: OutlinedButton(onPressed: _busy ? null : () => Navigator.of(context).pop(), child: Text(s.t('common.cancel')))),
                const SizedBox(width: 12),
                Expanded(
                  flex: 2,
                  child: FilledButton.icon(
                    onPressed: _cp != null && _photo != null && !_busy ? _submit : null,
                    icon: _busy
                        ? const SizedBox(width: 20, height: 20, child: CircularProgressIndicator(strokeWidth: 2.5, color: Colors.white))
                        : const Icon(Icons.check_rounded),
                    label: Text(s.t('pat2.manualSubmit')),
                  ),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}

class _CheckpointOption extends StatelessWidget {
  const _CheckpointOption({required this.index, required this.state, required this.now, required this.selected, required this.onTap});

  final int index;
  final CheckpointState state;
  final DateTime now;
  final bool selected;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final due = state.dueAt;
    final overdue = state.status == CpStatus.overdue;
    final sub = due == null
        ? s.t('pat2.noSla')
        : overdue
            ? '${s.t('pat.due', {'time': hhmm(due)})} · ${s.t('pat.overdue', {'n': minutesOver(due, now)})}'
            : s.t('pat.due', {'time': hhmm(due)});
    return Material(
      color: selected ? AppColors.primary50 : AppColors.surface,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(14), side: BorderSide(color: selected ? AppColors.primary : AppColors.border, width: selected ? 2 : 1)),
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: onTap,
        child: ConstrainedBox(
          constraints: const BoxConstraints(minHeight: 64),
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
            child: Row(
              children: [
                CheckpointMarker(index: index, state: state, size: 34),
                const SizedBox(width: 14),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(state.checkpoint.name, style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w600)),
                      const SizedBox(height: 2),
                      Text(sub, style: TextStyle(fontSize: 13.5, color: overdue ? AppColors.overdue : AppColors.textMuted, fontWeight: overdue ? FontWeight.w600 : FontWeight.w400)),
                    ],
                  ),
                ),
                Icon(selected ? Icons.radio_button_checked_rounded : Icons.radio_button_off_rounded, color: selected ? AppColors.primary : AppColors.textSubtle, size: 26),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
