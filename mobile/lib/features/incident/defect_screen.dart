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

/// Quick picks for the defect type; the label in the current language is
/// what gets sent (the API takes free text).
const _quickPicks = <(String, IconData)>[
  ('cctv', Icons.videocam_outlined),
  ('lamp', Icons.lightbulb_outline),
  ('door', Icons.door_front_door_outlined),
  ('fence', Icons.fence),
  ('power', Icons.electrical_services_outlined),
  ('water', Icons.water_drop_outlined),
  ('other', Icons.more_horiz_rounded),
];

/// Equipment / facility defect report (FR-INC-03): type, location,
/// description and one photo. Routed to the admin.
class DefectScreen extends StatefulWidget {
  const DefectScreen({super.key});

  @override
  State<DefectScreen> createState() => _DefectScreenState();
}

class _DefectScreenState extends State<DefectScreen> {
  final _type = TextEditingController();
  final _typeFocus = FocusNode();
  final _loc = TextEditingController();
  final _desc = TextEditingController();
  LocalPhoto? _photo;
  bool _taking = false;
  bool _busy = false;

  /// Quick pick last tapped ("other" only clears the field).
  String? _pick;

  @override
  void initState() {
    super.initState();
    for (final c in [_type, _loc, _desc]) {
      c.addListener(_changed);
    }
    _typeFocus.addListener(_changed);
  }

  @override
  void dispose() {
    _type.dispose();
    _typeFocus.dispose();
    _loc.dispose();
    _desc.dispose();
    super.dispose();
  }

  void _changed() => setState(() {});

  bool get _dirty => _type.text.trim().isNotEmpty || _loc.text.trim().isNotEmpty || _desc.text.trim().isNotEmpty || _photo != null;

  bool get _ready => _type.text.trim().isNotEmpty;

  void _touch() => context.read<DeviceSession>().touch();

  /// The quick pick matching the typed text, so the chip stays highlighted.
  String? _selectedPick(S s) {
    final typed = _type.text.trim().toLowerCase();
    for (final (code, _) in _quickPicks) {
      if (code != 'other' && s.t('rep2.def.q.$code').toLowerCase() == typed) return code;
    }
    return _pick == 'other' && _typeFocus.hasFocus ? 'other' : null;
  }

  void _choose(S s, String code) {
    _touch();
    _pick = code;
    if (code == 'other') {
      _type.clear();
      _typeFocus.requestFocus();
    } else {
      _type.text = s.t('rep2.def.q.$code');
      _typeFocus.unfocus();
    }
    setState(() {});
  }

  Future<void> _takePhoto() async {
    final s = S.of(context);
    final actions = context.read<FieldActions>();
    final siteId = context.read<DeviceSession>().site!.id;
    _touch();
    setState(() => _taking = true);
    try {
      final p = await actions.photos.take(kind: 'defect', siteId: siteId);
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
      await actions.reportDefect(type: _type.text, description: _desc.text, locationText: _loc.text, photo: _photo);
      if (!mounted) return;
      showSaved(context, 'def.saved');
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
    final selected = _selectedPick(s);

    return PopScope(
      canPop: !_dirty && !_busy,
      onPopInvokedWithResult: (didPop, _) {
        if (!didPop && !_busy) _leave();
      },
      child: Scaffold(
        appBar: AppBar(
          leading: IconButton(icon: const Icon(Icons.close_rounded), tooltip: s.t('common.close'), onPressed: () => Navigator.of(context).maybePop()),
          title: Text(s.t('def.title')),
        ),
        bottomNavigationBar: BottomActionBar(
          label: s.t('rep2.submit'),
          icon: Icons.send_rounded,
          busy: _busy,
          hint: s.t('rep2.def.need'),
          onPressed: _ready ? _submit : null,
        ),
        body: ReportBody(children: [
          ReporterStrip(subtitle: s.t('rep2.def.subtitle')),
          const SizedBox(height: 20),

          // 1 — what is broken
          SectionCard(
            child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              StepHeading(number: 1, title: s.t('rep2.def.what'), done: _ready),
              Wrap(
                spacing: 10,
                runSpacing: 10,
                children: [
                  for (final (code, icon) in _quickPicks) PickChip(label: s.t('rep2.def.q.$code'), icon: icon, selected: selected == code, onTap: () => _choose(s, code)),
                ],
              ),
              const SizedBox(height: 16),
              FieldLabel(s.t('def.type')),
              TextField(
                controller: _type,
                focusNode: _typeFocus,
                maxLength: 120,
                textCapitalization: TextCapitalization.sentences,
                textInputAction: TextInputAction.next,
                style: const TextStyle(fontSize: 16),
                decoration: InputDecoration(
                  hintText: s.t('def.typeHint'),
                  counterText: '',
                  hintStyle: const TextStyle(color: AppColors.textSubtle, fontSize: 15, fontWeight: FontWeight.w400),
                  prefixIcon: const Icon(Icons.build_outlined, color: AppColors.textMuted),
                ),
              ),
            ]),
          ),
          const SizedBox(height: 16),

          // 2 — where and what exactly
          SectionCard(
            child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              StepHeading(number: 2, title: s.t('rep2.details'), optional: true, done: _loc.text.trim().isNotEmpty || _desc.text.trim().isNotEmpty),
              ReportField(label: s.t('rep2.where'), controller: _loc, hint: s.t('rep2.def.locHint'), icon: Icons.place_outlined, maxLength: 200),
              const SizedBox(height: 16),
              ReportField(label: s.t('common.description'), controller: _desc, hint: s.t('rep2.def.descHint'), minLines: 3, maxLines: 6, maxLength: 2000),
            ]),
          ),
          const SizedBox(height: 16),

          // 3 — photo
          SectionCard(
            child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              StepHeading(number: 3, title: s.t('rep2.def.photo'), optional: true, done: _photo != null),
              PhotoStrip(
                photos: [if (_photo != null) _photo!],
                max: 1,
                busy: _taking,
                onAdd: _takePhoto,
                onRemove: _removePhoto,
                size: narrow ? 120 : 140,
              ),
            ]),
          ),
        ]),
      ),
    );
  }
}
