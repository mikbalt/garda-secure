import 'dart:async';

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../../core/auth/auth_provider.dart';
import '../../core/services/location_service.dart';
import '../../core/services/photo_service.dart';
import '../../l10n/strings.dart';
import '../../shared/widgets/ui.dart';
import '../../theme/app_theme.dart';
import '../field_actions.dart';
import 'report_kit.dart';

/// Incident categories (API `type`) with their icons, in the order shown.
const incidentCategories = <(String, IconData)>[
  ('theft', Icons.no_backpack_outlined),
  ('trespassing', Icons.fence),
  ('suspicious', Icons.person_search_outlined),
  ('vandalism', Icons.format_paint_outlined),
  ('fire_hazard', Icons.local_fire_department_outlined),
  ('medical', Icons.medical_services_outlined),
  ('accident', Icons.car_crash_outlined),
  ('other', Icons.more_horiz_rounded),
];

/// Severity levels (API `severity`) and their colours.
const incidentSeverities = <(String, Color)>[
  ('low', AppColors.textMuted),
  ('medium', AppColors.warn),
  ('high', AppColors.overdue),
  ('critical', AppColors.danger),
];

const _maxPhotos = 5;

/// Guided incident report (FR-INC-01): category, severity, details, up to five
/// photos. Queued in the outbox, so it works offline.
class IncidentScreen extends StatefulWidget {
  const IncidentScreen({super.key});

  @override
  State<IncidentScreen> createState() => _IncidentScreenState();
}

class _IncidentScreenState extends State<IncidentScreen> {
  String? _type;
  String? _severity;
  final _desc = TextEditingController();
  final _loc = TextEditingController();
  final _photos = <LocalPhoto>[];
  bool _taking = false;
  bool _busy = false;
  late final Future<Fix?> _fix;

  @override
  void initState() {
    super.initState();
    // GPS is best-effort and must never hold the guard up: start now, use
    // whatever is there when the report is sent.
    _fix = context.read<FieldActions>().location.current();
    _desc.addListener(_changed);
    _loc.addListener(_changed);
  }

  @override
  void dispose() {
    _desc.dispose();
    _loc.dispose();
    super.dispose();
  }

  void _changed() => setState(() {});

  bool get _dirty => _type != null || _severity != null || _desc.text.trim().isNotEmpty || _loc.text.trim().isNotEmpty || _photos.isNotEmpty;

  bool get _ready => _type != null && _severity != null;

  void _touch() => context.read<DeviceSession>().touch();

  Future<void> _addPhoto() async {
    final s = S.of(context);
    if (_photos.length >= _maxPhotos) {
      showMessage(context, s.t('rep2.photo.max', {'n': _maxPhotos}), error: true);
      return;
    }
    final actions = context.read<FieldActions>();
    final siteId = context.read<DeviceSession>().site!.id;
    _touch();
    setState(() => _taking = true);
    try {
      final p = await actions.photos.take(kind: 'incident', siteId: siteId);
      if (p != null && mounted) setState(() => _photos.add(p));
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
    setState(() => _photos.remove(p));
    unawaited(context.read<FieldActions>().photos.discard(p));
  }

  Future<void> _submit() async {
    if (!_ready || _busy) return;
    final actions = context.read<FieldActions>();
    _touch();
    setState(() => _busy = true);
    try {
      final fix = await _fix.timeout(const Duration(seconds: 2), onTimeout: () => null);
      await actions.reportIncident(
        type: _type!,
        severity: _severity!,
        description: _desc.text,
        locationText: _loc.text,
        photos: List.of(_photos),
        fix: fix,
      );
      if (!mounted) return;
      showSaved(context, 'inc.saved');
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
    for (final p in _photos) {
      unawaited(photos.discard(p));
    }
    nav.pop();
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final width = MediaQuery.sizeOf(context).width;
    final narrow = width < 600;
    final escalates = _severity == 'high' || _severity == 'critical';
    final sevColor = incidentSeverities.where((e) => e.$1 == _severity).map((e) => e.$2).firstOrNull;

    return PopScope(
      canPop: !_dirty && !_busy,
      onPopInvokedWithResult: (didPop, _) {
        if (!didPop && !_busy) _leave();
      },
      child: Scaffold(
        appBar: AppBar(
          leading: IconButton(icon: const Icon(Icons.close_rounded), tooltip: s.t('common.close'), onPressed: () => Navigator.of(context).maybePop()),
          title: Text(s.t('inc.title')),
        ),
        bottomNavigationBar: BottomActionBar(
          label: s.t('rep2.submit'),
          icon: Icons.send_rounded,
          busy: _busy,
          hint: s.t('rep2.inc.need'),
          onPressed: _ready ? _submit : null,
        ),
        body: ReportBody(children: [
          ReporterStrip(subtitle: s.t('rep2.inc.subtitle')),
          const SizedBox(height: 20),

          // 1 — category
          SectionCard(
            child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              StepHeading(number: 1, title: s.t('rep2.inc.what'), done: _type != null),
              TileGrid(
                columns: narrow ? 2 : 4,
                children: [
                  for (final (code, icon) in incidentCategories)
                    ChoiceTile(
                      label: s.t('inc.cat.$code'),
                      icon: icon,
                      selected: _type == code,
                      height: narrow ? 92 : 100,
                      onTap: () => setState(() => _type = code),
                    ),
                ],
              ),
            ]),
          ),
          const SizedBox(height: 16),

          // 2 — severity
          SectionCard(
            child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              StepHeading(number: 2, title: s.t('rep2.inc.how'), done: _severity != null),
              TileGrid(
                columns: narrow ? 2 : 4,
                children: [
                  for (final (code, color) in incidentSeverities)
                    ChoiceTile(
                      label: s.t('sev.$code'),
                      caption: s.t('rep2.sev.d.$code'),
                      dotColor: color,
                      color: color,
                      selected: _severity == code,
                      height: 100,
                      onTap: () => setState(() => _severity = code),
                    ),
                ],
              ),
              const SizedBox(height: 12),
              AnimatedContainer(
                duration: const Duration(milliseconds: 200),
                padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
                decoration: BoxDecoration(
                  color: escalates ? sevColor!.withValues(alpha: 0.08) : AppColors.bg,
                  borderRadius: BorderRadius.circular(12),
                  border: Border.all(color: escalates ? sevColor!.withValues(alpha: 0.35) : AppColors.border),
                ),
                child: Row(children: [
                  Icon(escalates ? Icons.notifications_active_outlined : Icons.info_outline, size: 20, color: escalates ? Color.lerp(sevColor, Colors.black, 0.2) : AppColors.textMuted),
                  const SizedBox(width: 10),
                  Expanded(
                    child: Text(s.t('sev.hint'),
                        style: TextStyle(fontSize: 14, fontWeight: escalates ? FontWeight.w600 : FontWeight.w400, color: escalates ? Color.lerp(sevColor, Colors.black, 0.35) : AppColors.textMuted)),
                  ),
                ]),
              ),
            ]),
          ),
          const SizedBox(height: 16),

          // 3 — details
          SectionCard(
            child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              StepHeading(number: 3, title: s.t('rep2.details'), optional: true, done: _desc.text.trim().isNotEmpty),
              ReportField(label: s.t('common.description'), controller: _desc, hint: s.t('rep2.inc.descHint'), minLines: 4, maxLines: 8, maxLength: 2000),
              const SizedBox(height: 16),
              ReportField(label: s.t('rep2.where'), controller: _loc, hint: s.t('rep2.locHint'), icon: Icons.place_outlined, maxLength: 200, action: TextInputAction.done),
            ]),
          ),
          const SizedBox(height: 16),

          // 4 — photos
          SectionCard(
            child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              StepHeading(
                number: 4,
                title: s.t('common.photo'),
                optional: true,
                done: _photos.isNotEmpty,
                trailing: Text(s.t('rep2.photo.count', {'n': _photos.length, 'max': _maxPhotos}), style: const TextStyle(fontSize: 13.5, color: AppColors.textMuted)),
              ),
              PhotoStrip(photos: _photos, max: _maxPhotos, busy: _taking, onAdd: _addPhoto, onRemove: _removePhoto, size: narrow ? 96 : 112),
            ]),
          ),
        ]),
      ),
    );
  }
}
