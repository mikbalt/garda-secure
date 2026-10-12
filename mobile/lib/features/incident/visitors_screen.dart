import 'dart:async';

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../../core/auth/auth_provider.dart';
import '../../core/services/photo_service.dart';
import '../../l10n/strings.dart';
import '../../shared/widgets/ui.dart';
import '../../theme/app_theme.dart';
import '../field_actions.dart';
import 'report_kit.dart';
import '../../core/services/clock.dart';

/// Visitor / vehicle log of this tablet (FR-INC-04): who is still inside,
/// recording exits, and the "visitor in" form.
class VisitorsScreen extends StatefulWidget {
  const VisitorsScreen({super.key});

  @override
  State<VisitorsScreen> createState() => _VisitorsScreenState();
}

class _VisitorsScreenState extends State<VisitorsScreen> {
  Timer? _tick;

  @override
  void initState() {
    super.initState();
    // Keeps the "time inside" durations current.
    _tick = Timer.periodic(const Duration(seconds: 30), (_) => setState(() {}));
  }

  @override
  void dispose() {
    _tick?.cancel();
    super.dispose();
  }

  Future<void> _newVisitor() async {
    context.read<DeviceSession>().touch();
    await Navigator.of(context).push(MaterialPageRoute<bool>(builder: (_) => const VisitorInScreen()));
  }

  Future<void> _exit(LocalVisitor v) async {
    final s = S.of(context);
    final actions = context.read<FieldActions>();
    context.read<DeviceSession>().touch();
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        icon: const Icon(Icons.logout_rounded, color: AppColors.primary, size: 32),
        title: Text(s.t('rep2.vis.exitTitle', {'name': v.name})),
        content: Text(s.t('rep2.vis.exitBody', {'time': hhmm(AppClock.now())}), textAlign: TextAlign.center),
        actionsPadding: const EdgeInsets.fromLTRB(16, 0, 16, 16),
        actions: [
          OutlinedButton(onPressed: () => Navigator.of(ctx).pop(false), child: Text(s.t('common.cancel'))),
          FilledButton.icon(onPressed: () => Navigator.of(ctx).pop(true), icon: const Icon(Icons.logout_rounded), label: Text(s.t('vis.exit'))),
        ],
      ),
    );
    if (ok != true || !mounted) return;
    try {
      await actions.visitorOut(v);
      if (mounted) showSaved(context, 'vis.exited');
    } catch (_) {
      if (mounted) showMessage(context, s.t('common.error'), error: true);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final all = context.watch<FieldActions>().visitors;
    final inside = all.where((v) => v.inside).toList()..sort((a, b) => b.entryAt.compareTo(a.entryAt));
    final left = all.where((v) => !v.inside).toList()..sort((a, b) => b.exitAt!.compareTo(a.exitAt!));
    final width = MediaQuery.sizeOf(context).width;
    final narrow = width < 600;
    final columns = width >= 900 ? 2 : 1;
    final now = AppClock.now();

    return Scaffold(
      appBar: AppBar(title: Text(s.t('vis.title'))),
      bottomNavigationBar: narrow && all.isNotEmpty ? BottomActionBar(label: s.t('vis.new'), icon: Icons.person_add_alt_1_outlined, onPressed: _newVisitor) : null,
      body: Listener(
        behavior: HitTestBehavior.translucent,
        onPointerDown: (_) => context.read<DeviceSession>().touch(),
        child: SingleChildScrollView(
          padding: EdgeInsets.fromLTRB(narrow ? 16 : 24, narrow ? 16 : 24, narrow ? 16 : 24, 32),
          child: Constrained(
            maxWidth: 1100,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                if (all.isNotEmpty) ...[
                  Row(children: [
                    const IconTile(icon: Icons.badge_outlined, color: AppColors.primary, size: 48),
                    const SizedBox(width: 14),
                    Expanded(
                      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                        Text(s.t('rep2.vis.summary', {'n': inside.length}), style: Theme.of(context).textTheme.titleLarge),
                        const SizedBox(height: 2),
                        Text(s.t('rep2.vis.recorded', {'n': all.length}), style: const TextStyle(fontSize: 14, color: AppColors.textMuted)),
                      ]),
                    ),
                    if (!narrow)
                      SizedBox(
                        height: 56,
                        child: FilledButton.icon(
                          onPressed: _newVisitor,
                          icon: const Icon(Icons.person_add_alt_1_outlined),
                          label: Text(s.t('vis.new')),
                        ),
                      ),
                  ]),
                  const SizedBox(height: 20),
                ],
                if (all.isEmpty)
                  _Empty(onNew: _newVisitor)
                else ...[
                  if (inside.isNotEmpty) ...[
                    _GroupLabel(text: s.t('rep2.vis.insideN', {'n': inside.length}), color: AppColors.ok),
                    TileGrid(
                      columns: columns,
                      spacing: 12,
                      children: [for (final v in inside) _VisitorCard(visitor: v, now: now, onExit: () => _exit(v))],
                    ),
                    const SizedBox(height: 24),
                  ],
                  if (left.isNotEmpty) ...[
                    _GroupLabel(text: s.t('rep2.vis.leftN', {'n': left.length}), color: AppColors.textSubtle),
                    TileGrid(
                      columns: columns,
                      spacing: 12,
                      children: [for (final v in left) _VisitorCard(visitor: v, now: now)],
                    ),
                  ],
                ],
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _GroupLabel extends StatelessWidget {
  const _GroupLabel({required this.text, required this.color});

  final String text;
  final Color color;

  @override
  Widget build(BuildContext context) => Padding(
        padding: const EdgeInsets.only(bottom: 10, left: 2),
        child: Row(children: [
          Container(width: 8, height: 8, decoration: BoxDecoration(color: color, shape: BoxShape.circle)),
          const SizedBox(width: 8),
          Text(text.toUpperCase(), style: const TextStyle(fontSize: 12.5, fontWeight: FontWeight.w700, letterSpacing: 0.6, color: AppColors.textMuted)),
        ]),
      );
}

class _Empty extends StatelessWidget {
  const _Empty({required this.onNew});

  final VoidCallback onNew;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    return SectionCard(
      padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 40),
      child: Column(children: [
        const IconTile(icon: Icons.groups_outlined, color: AppColors.textMuted, size: 72),
        const SizedBox(height: 16),
        Text(s.t('vis.empty'), textAlign: TextAlign.center, style: Theme.of(context).textTheme.titleMedium),
        const SizedBox(height: 6),
        Text(s.t('rep2.vis.emptyHint'), textAlign: TextAlign.center, style: const TextStyle(fontSize: 14, color: AppColors.textMuted)),
        const SizedBox(height: 20),
        SizedBox(
          height: 56,
          child: FilledButton.icon(onPressed: onNew, icon: const Icon(Icons.person_add_alt_1_outlined), label: Text(s.t('vis.new'))),
        ),
      ]),
    );
  }
}

class _VisitorCard extends StatelessWidget {
  const _VisitorCard({required this.visitor, required this.now, this.onExit});

  final LocalVisitor visitor;
  final DateTime now;
  final VoidCallback? onExit;

  String _initials(String name) {
    final parts = name.trim().split(RegExp(r'\s+')).where((p) => p.isNotEmpty).toList();
    if (parts.isEmpty) return '?';
    return (parts.first[0] + (parts.length > 1 ? parts[1][0] : '')).toUpperCase();
  }

  /// "09:12", or "08/10 09:12" when not today.
  String _when(DateTime t) {
    final l = AppClock.site(t);
    final today = AppClock.siteNow();
    final sameDay = l.year == today.year && l.month == today.month && l.day == today.day;
    return sameDay ? hhmm(t) : '${l.day.toString().padLeft(2, '0')}/${l.month.toString().padLeft(2, '0')} ${hhmm(t)}';
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final v = visitor;
    final stay = (v.exitAt ?? now).difference(v.entryAt);
    final tt = Theme.of(context).textTheme;
    return SectionCard(
      padding: const EdgeInsets.all(16),
      color: v.inside ? null : AppColors.bg,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          LayoutBuilder(builder: (context, c) {
            final pill = v.inside ? Pill(text: s.t('vis.inside'), color: AppColors.ok, dot: true) : Pill(text: s.t('rep2.vis.out', {'time': _when(v.exitAt!)}), color: AppColors.textMuted);
            // Narrow cards: status under the name so long names stay readable.
            final stacked = c.maxWidth < 440;
            return Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Avatar(initials: _initials(v.name), size: 46, color: v.inside ? AppColors.primary : AppColors.textSubtle),
              const SizedBox(width: 12),
              Expanded(
                child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                  Text(v.name, style: tt.titleMedium?.copyWith(fontSize: 17), maxLines: 2, overflow: TextOverflow.ellipsis),
                  if (v.purpose != null) ...[
                    const SizedBox(height: 2),
                    Text(v.purpose!, style: tt.bodySmall?.copyWith(fontSize: 14), maxLines: 2, overflow: TextOverflow.ellipsis),
                  ],
                  if (stacked) ...[const SizedBox(height: 8), pill],
                ]),
              ),
              if (!stacked) ...[const SizedBox(width: 8), pill],
            ]);
          }),
          const SizedBox(height: 14),
          Wrap(spacing: 18, runSpacing: 8, children: [
            _Meta(icon: Icons.login_rounded, text: s.t('rep2.vis.in', {'time': _when(v.entryAt)})),
            _Meta(icon: Icons.timelapse_rounded, text: duration(s, stay), strong: v.inside),
            if (v.plate != null) _Meta(icon: Icons.directions_car_outlined, text: v.plate!, strong: true),
            if (v.host != null) _Meta(icon: Icons.person_outline, text: v.host!),
          ]),
          if (onExit != null) ...[
            const SizedBox(height: 14),
            SizedBox(
              height: 52,
              child: OutlinedButton.icon(
                onPressed: onExit,
                style: OutlinedButton.styleFrom(foregroundColor: AppColors.primary700, side: const BorderSide(color: AppColors.primary100, width: 1.5)),
                icon: const Icon(Icons.logout_rounded),
                label: Text(s.t('vis.exit')),
              ),
            ),
          ],
        ],
      ),
    );
  }
}

class _Meta extends StatelessWidget {
  const _Meta({required this.icon, required this.text, this.strong = false});

  final IconData icon;
  final String text;
  final bool strong;

  @override
  Widget build(BuildContext context) => Row(mainAxisSize: MainAxisSize.min, children: [
        Icon(icon, size: 18, color: AppColors.textSubtle),
        const SizedBox(width: 6),
        Text(text, style: TextStyle(fontSize: 14, fontWeight: strong ? FontWeight.w600 : FontWeight.w400, color: strong ? AppColors.text : AppColors.textMuted)),
      ]);
}

/// ID types offered on the form. The stored value is the same in both
/// languages (the dashboard shows it as is, e.g. "KTP 3216…").
const visitorIdTypes = <(String, String)>[
  ('KTP', 'ktp'),
  ('SIM', 'sim'),
  ('Paspor', 'passport'),
  ('Lainnya', 'other'),
];

/// "Visitor in" form: only the name is required.
class VisitorInScreen extends StatefulWidget {
  const VisitorInScreen({super.key});

  @override
  State<VisitorInScreen> createState() => _VisitorInScreenState();
}

class _VisitorInScreenState extends State<VisitorInScreen> {
  final _name = TextEditingController();
  final _idNumber = TextEditingController();
  final _plate = TextEditingController();
  final _host = TextEditingController();
  final _purpose = TextEditingController();
  String? _idType = 'KTP';

  /// The preselected ID type is only sent with a number or an explicit tap.
  bool _idTypeChosen = false;
  LocalPhoto? _photo;
  bool _taking = false;
  bool _busy = false;

  List<TextEditingController> get _fields => [_name, _idNumber, _plate, _host, _purpose];

  @override
  void initState() {
    super.initState();
    for (final c in _fields) {
      c.addListener(_changed);
    }
  }

  @override
  void dispose() {
    for (final c in _fields) {
      c.dispose();
    }
    super.dispose();
  }

  void _changed() => setState(() {});

  bool get _dirty => _fields.any((c) => c.text.trim().isNotEmpty) || _photo != null;

  bool get _ready => _name.text.trim().isNotEmpty;

  void _touch() => context.read<DeviceSession>().touch();

  Future<void> _takePhoto() async {
    final s = S.of(context);
    final actions = context.read<FieldActions>();
    final siteId = context.read<DeviceSession>().site!.id;
    _touch();
    setState(() => _taking = true);
    try {
      final p = await actions.photos.take(kind: 'visitor', siteId: siteId);
      if (p != null && mounted) {
        final old = _photo;
        setState(() => _photo = p);
        if (old != null) unawaited(actions.photos.discard(old));
      }
    } catch (_) {
      if (mounted) showMessage(context, s.t('rep2.photo.failed'), error: true);
    } finally {
      if (mounted) {
        setState(() => _taking = false);
        _touch();
      }
    }
  }

  void _removePhoto(LocalPhoto p) {
    setState(() => _photo = null);
    unawaited(context.read<FieldActions>().photos.discard(p));
  }

  Future<void> _submit() async {
    if (!_ready || _busy) return;
    final actions = context.read<FieldActions>();
    _touch();
    setState(() => _busy = true);
    try {
      await actions.visitorIn(
        name: _name.text,
        idType: _idNumber.text.trim().isNotEmpty || _idTypeChosen ? _idType : null,
        idNumber: _idNumber.text,
        plate: _plate.text,
        host: _host.text,
        purpose: _purpose.text,
        photo: _photo,
      );
      if (!mounted) return;
      showSaved(context, 'vis.saved');
      Navigator.of(context).pop(true);
    } catch (_) {
      if (!mounted) return;
      setState(() => _busy = false);
      showMessage(context, S.of(context).t('common.error'), error: true);
    }
  }

  Future<void> _leave() async {
    final photos = context.read<FieldActions>().photos;
    final nav = Navigator.of(context);
    if (!await confirmDiscard(context)) return;
    if (_photo != null) unawaited(photos.discard(_photo!));
    nav.pop();
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final narrow = MediaQuery.sizeOf(context).width < 600;

    Widget pair(Widget a, Widget b) => narrow
        ? Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [a, const SizedBox(height: 16), b])
        : Row(crossAxisAlignment: CrossAxisAlignment.start, children: [Expanded(child: a), const SizedBox(width: 16), Expanded(child: b)]);

    return PopScope(
      canPop: !_dirty && !_busy,
      onPopInvokedWithResult: (didPop, _) {
        if (!didPop && !_busy) _leave();
      },
      child: Scaffold(
        appBar: AppBar(
          leading: IconButton(icon: const Icon(Icons.close_rounded), tooltip: s.t('common.close'), onPressed: () => Navigator.of(context).maybePop()),
          title: Text(s.t('vis.new')),
        ),
        bottomNavigationBar: BottomActionBar(
          label: s.t('rep2.vis.submit'),
          icon: Icons.how_to_reg_outlined,
          busy: _busy,
          hint: s.t('rep2.vis.need'),
          onPressed: _ready ? _submit : null,
        ),
        body: ReportBody(children: [
          ReporterStrip(subtitle: s.t('rep2.vis.formSub')),
          const SizedBox(height: 20),

          // 1 — who
          SectionCard(
            child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              StepHeading(number: 1, title: s.t('rep2.vis.who'), done: _ready),
              ReportField(label: s.t('vis.name'), controller: _name, hint: s.t('rep2.vis.nameHint'), icon: Icons.person_outline, caps: TextCapitalization.words, maxLength: 120),
              const SizedBox(height: 16),
              FieldLabel(s.t('vis.idType'), optional: s.t('common.optional')),
              Wrap(spacing: 10, runSpacing: 10, children: [
                for (final (value, key) in visitorIdTypes)
                  PickChip(
                    label: s.t('rep2.vis.id.$key'),
                    selected: _idType == value,
                    onTap: () {
                      _touch();
                      setState(() {
                        _idTypeChosen = true;
                        _idType = _idType == value ? null : value;
                      });
                    },
                  ),
              ]),
              const SizedBox(height: 16),
              ReportField(
                label: s.t('vis.idNumber'),
                controller: _idNumber,
                hint: s.t('rep2.vis.idHint'),
                optional: true,
                icon: Icons.badge_outlined,
                caps: TextCapitalization.characters,
                maxLength: 40,
              ),
            ]),
          ),
          const SizedBox(height: 16),

          // 2 — visit
          SectionCard(
            child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              StepHeading(number: 2, title: s.t('rep2.vis.visit'), optional: true, done: _host.text.trim().isNotEmpty || _purpose.text.trim().isNotEmpty),
              pair(
                ReportField(label: s.t('vis.host'), controller: _host, hint: s.t('rep2.vis.hostHint'), icon: Icons.apartment_outlined, caps: TextCapitalization.words, maxLength: 120),
                ReportField(label: s.t('vis.plate'), controller: _plate, hint: s.t('rep2.vis.plateHint'), icon: Icons.directions_car_outlined, caps: TextCapitalization.characters, maxLength: 20),
              ),
              const SizedBox(height: 16),
              ReportField(label: s.t('vis.purpose'), controller: _purpose, hint: s.t('rep2.vis.purposeHint'), icon: Icons.assignment_outlined, maxLength: 200, action: TextInputAction.done),
            ]),
          ),
          const SizedBox(height: 16),

          // 3 — photo
          SectionCard(
            child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              StepHeading(number: 3, title: s.t('common.photo'), optional: true, done: _photo != null),
              Text(s.t('vis.photoHint'), style: const TextStyle(fontSize: 14, color: AppColors.textMuted)),
              const SizedBox(height: 12),
              PhotoStrip(photos: [if (_photo != null) _photo!], max: 1, busy: _taking, onAdd: _takePhoto, onRemove: _removePhoto, size: narrow ? 120 : 140),
            ]),
          ),
        ]),
      ),
    );
  }
}
