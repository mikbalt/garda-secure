import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../../core/auth/auth_provider.dart';
import '../../core/database/sync_service.dart';
import '../../core/services/photo_service.dart';
import '../../l10n/strings.dart';
import '../../shared/widgets/ui.dart';
import '../../theme/app_theme.dart';

/// Building blocks shared by the incident, defect and visitor forms.

/// Scrollable, centred form body that keeps the idle auto-lock awake while
/// the guard is typing or tapping.
class ReportBody extends StatelessWidget {
  const ReportBody({super.key, required this.children, this.maxWidth = 760});

  final List<Widget> children;
  final double maxWidth;

  @override
  Widget build(BuildContext context) {
    final narrow = MediaQuery.sizeOf(context).width < 600;
    return Listener(
      behavior: HitTestBehavior.translucent,
      onPointerDown: (_) => context.read<DeviceSession>().touch(),
      child: SingleChildScrollView(
        keyboardDismissBehavior: ScrollViewKeyboardDismissBehavior.onDrag,
        padding: EdgeInsets.fromLTRB(narrow ? 16 : 24, narrow ? 16 : 24, narrow ? 16 : 24, 32),
        child: Constrained(
          maxWidth: maxWidth,
          child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: children),
        ),
      ),
    );
  }
}

/// Numbered step heading; the number turns into a green tick once done.
class StepHeading extends StatelessWidget {
  const StepHeading({super.key, required this.number, required this.title, this.done = false, this.optional = false, this.trailing});

  final int number;
  final String title;
  final bool done;
  final bool optional;
  final Widget? trailing;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    return Padding(
      padding: const EdgeInsets.only(bottom: 14),
      child: Row(
        children: [
          AnimatedContainer(
            duration: const Duration(milliseconds: 200),
            width: 30,
            height: 30,
            alignment: Alignment.center,
            decoration: BoxDecoration(color: done ? AppColors.ok : AppColors.primary50, shape: BoxShape.circle),
            child:
                done ? const Icon(Icons.check_rounded, size: 18, color: Colors.white) : Text('$number', style: const TextStyle(fontSize: 14, fontWeight: FontWeight.w700, color: AppColors.primary700)),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Text.rich(TextSpan(children: [
              TextSpan(text: title, style: const TextStyle(fontSize: 17, fontWeight: FontWeight.w600, color: AppColors.text)),
              if (optional) TextSpan(text: '  (${s.t('common.optional')})', style: const TextStyle(fontSize: 14, color: AppColors.textSubtle)),
            ])),
          ),
          if (trailing != null) trailing!,
        ],
      ),
    );
  }
}

/// Who and where the report is filed under (the tablet is shared).
class ReporterStrip extends StatelessWidget {
  const ReporterStrip({super.key, this.subtitle});

  final String? subtitle;

  @override
  Widget build(BuildContext context) {
    final session = context.watch<DeviceSession>();
    final now = DateTime.now();
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        if (subtitle != null) ...[
          Text(subtitle!, style: const TextStyle(fontSize: 15, color: AppColors.textMuted, height: 1.4)),
          const SizedBox(height: 12),
        ],
        Wrap(spacing: 8, runSpacing: 8, children: [
          if (session.guard != null) Pill(text: session.guard!.name, color: AppColors.primary, icon: Icons.person_outline),
          if (session.site != null) Pill(text: session.site!.name, color: AppColors.textMuted, icon: Icons.place_outlined),
          Pill(text: hhmm(now), color: AppColors.textMuted, icon: Icons.schedule),
        ]),
      ],
    );
  }
}

/// Large selectable tile: icon above a label, filled when selected.
class ChoiceTile extends StatelessWidget {
  const ChoiceTile({super.key, required this.label, required this.selected, required this.onTap, this.icon, this.caption, this.color = AppColors.primary, this.height = 96, this.dotColor});

  final String label;
  final bool selected;
  final VoidCallback onTap;
  final IconData? icon;
  final String? caption;
  final Color color;
  final double height;

  /// Shows a coloured dot instead of an icon (severity).
  final Color? dotColor;

  @override
  Widget build(BuildContext context) {
    final fg = selected ? Colors.white : AppColors.text;
    return Semantics(
      selected: selected,
      button: true,
      child: Container(
        height: height,
        decoration: BoxDecoration(
          color: selected ? color : AppColors.surface,
          borderRadius: BorderRadius.circular(14),
          border: Border.all(color: selected ? color : (dotColor != null ? color.withOpacity(0.45) : AppColors.border), width: selected ? 2 : 1.2),
          boxShadow: selected ? [BoxShadow(color: color.withOpacity(0.25), blurRadius: 10, offset: const Offset(0, 3))] : null,
        ),
        child: Material(
          type: MaterialType.transparency,
          child: InkWell(
            borderRadius: BorderRadius.circular(14),
            onTap: onTap,
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 8),
              child: Column(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  if (icon != null) Icon(icon, size: 32, color: selected ? Colors.white : color),
                  if (dotColor != null)
                    Container(
                      width: 16,
                      height: 16,
                      decoration: BoxDecoration(color: selected ? Colors.white : dotColor, shape: BoxShape.circle, border: selected ? null : Border.all(color: Colors.white, width: 2)),
                    ),
                  const SizedBox(height: 8),
                  Text(label, textAlign: TextAlign.center, maxLines: 2, overflow: TextOverflow.ellipsis, style: TextStyle(fontSize: 15, fontWeight: FontWeight.w600, color: fg, height: 1.2)),
                  if (caption != null) ...[
                    const SizedBox(height: 2),
                    Text(caption!,
                        textAlign: TextAlign.center,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: TextStyle(fontSize: 12.5, color: selected ? Colors.white.withOpacity(0.9) : AppColors.textMuted)),
                  ],
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

/// Lays [children] out in equal columns ([columns] per row).
class TileGrid extends StatelessWidget {
  const TileGrid({super.key, required this.columns, required this.children, this.spacing = 10});

  final int columns;
  final List<Widget> children;
  final double spacing;

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(builder: (context, c) {
      final w = (c.maxWidth - spacing * (columns - 1)) / columns;
      return Wrap(
        spacing: spacing,
        runSpacing: spacing,
        children: [for (final child in children) SizedBox(width: w.floorToDouble(), child: child)],
      );
    });
  }
}

/// Pill-shaped quick-pick button with an icon (min. 52 px tall).
class PickChip extends StatelessWidget {
  const PickChip({super.key, required this.label, required this.selected, required this.onTap, this.icon});

  final String label;
  final IconData? icon;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Material(
      color: selected ? AppColors.primary : AppColors.surface,
      shape: StadiumBorder(side: BorderSide(color: selected ? AppColors.primary : const Color(0xFFCBD5E1), width: 1.2)),
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: onTap,
        child: ConstrainedBox(
          constraints: const BoxConstraints(minHeight: 52),
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 12),
            child: Row(mainAxisSize: MainAxisSize.min, children: [
              if (icon != null) ...[
                Icon(icon, size: 22, color: selected ? Colors.white : AppColors.primary),
                const SizedBox(width: 8),
              ],
              Text(label, style: TextStyle(fontSize: 15.5, fontWeight: FontWeight.w600, color: selected ? Colors.white : AppColors.text)),
            ]),
          ),
        ),
      ),
    );
  }
}

/// Row of photo thumbnails plus an "add" tile while below [max].
class PhotoStrip extends StatelessWidget {
  const PhotoStrip({super.key, required this.photos, required this.max, required this.onAdd, required this.onRemove, this.busy = false, this.addLabel, this.size = 104});

  final List<LocalPhoto> photos;
  final int max;
  final VoidCallback onAdd;
  final ValueChanged<LocalPhoto> onRemove;
  final bool busy;
  final String? addLabel;
  final double size;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    return Wrap(
      spacing: 10,
      runSpacing: 10,
      children: [
        for (final p in photos) PhotoThumb(path: p.path, size: size, onRemove: () => onRemove(p)),
        if (photos.length < max)
          busy
              ? SizedBox(
                  width: size,
                  height: size,
                  child: const DecoratedBox(
                    decoration: BoxDecoration(color: AppColors.primary50, borderRadius: BorderRadius.all(Radius.circular(14))),
                    child: Center(child: SizedBox(width: 26, height: 26, child: CircularProgressIndicator(strokeWidth: 2.5))),
                  ),
                )
              : PhotoThumb(size: size, label: addLabel ?? s.t('rep2.photo.add'), onTap: onAdd),
      ],
    );
  }
}

/// Sticky bottom bar with the form's main action, always in thumb reach.
class BottomActionBar extends StatelessWidget {
  const BottomActionBar({super.key, required this.label, required this.icon, required this.onPressed, this.busy = false, this.hint});

  final String label;
  final IconData icon;
  final VoidCallback? onPressed;
  final bool busy;

  /// Shown above the button while it is disabled (what is still missing).
  final String? hint;

  @override
  Widget build(BuildContext context) {
    final narrow = MediaQuery.sizeOf(context).width < 600;
    return Container(
      decoration: const BoxDecoration(color: AppColors.surface, border: Border(top: BorderSide(color: AppColors.border))),
      child: SafeArea(
        top: false,
        child: Padding(
          padding: EdgeInsets.fromLTRB(narrow ? 16 : 24, 12, narrow ? 16 : 24, 14),
          child: Center(
            heightFactor: 1,
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 760),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  if (hint != null && onPressed == null && !busy)
                    Padding(
                      padding: const EdgeInsets.only(bottom: 10),
                      child: Row(mainAxisAlignment: MainAxisAlignment.center, children: [
                        const Icon(Icons.info_outline, size: 18, color: AppColors.textMuted),
                        const SizedBox(width: 6),
                        Flexible(child: Text(hint!, style: const TextStyle(fontSize: 14, color: AppColors.textMuted))),
                      ]),
                    ),
                  SizedBox(
                    height: 60,
                    child: FilledButton.icon(
                      onPressed: busy ? null : onPressed,
                      style: FilledButton.styleFrom(textStyle: const TextStyle(fontFamily: 'Inter', fontSize: 17, fontWeight: FontWeight.w600)),
                      icon: busy ? const SizedBox(width: 22, height: 22, child: CircularProgressIndicator(strokeWidth: 2.5, color: Colors.white)) : Icon(icon, size: 22),
                      label: Text(label),
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

/// Label + text field with a hint, sized for gloved fingers.
class ReportField extends StatelessWidget {
  const ReportField({
    super.key,
    required this.label,
    required this.controller,
    this.hint,
    this.optional = false,
    this.icon,
    this.minLines = 1,
    this.maxLines = 1,
    this.caps = TextCapitalization.sentences,
    this.keyboard,
    this.action = TextInputAction.next,
    this.maxLength,
  });

  final String label;
  final TextEditingController controller;
  final String? hint;
  final bool optional;
  final IconData? icon;
  final int minLines;
  final int maxLines;
  final TextCapitalization caps;
  final TextInputType? keyboard;
  final TextInputAction action;
  final int? maxLength;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        FieldLabel(label, optional: optional ? s.t('common.optional') : null),
        TextField(
          controller: controller,
          minLines: minLines,
          maxLines: maxLines,
          maxLength: maxLength,
          textCapitalization: caps,
          keyboardType: keyboard ?? (maxLines > 1 ? TextInputType.multiline : TextInputType.text),
          textInputAction: maxLines > 1 ? TextInputAction.newline : action,
          style: const TextStyle(fontSize: 16),
          decoration: InputDecoration(
            hintText: hint,
            hintMaxLines: 3,
            counterText: '',
            hintStyle: const TextStyle(color: AppColors.textSubtle, fontSize: 15, fontWeight: FontWeight.w400),
            prefixIcon: icon == null ? null : Icon(icon, color: AppColors.textMuted),
          ),
        ),
      ],
    );
  }
}

/// Asks before throwing away a half-filled form. True = discard.
Future<bool> confirmDiscard(BuildContext context) async {
  final s = S.of(context);
  final ok = await showDialog<bool>(
    context: context,
    builder: (ctx) => AlertDialog(
      title: Text(s.t('rep2.discard.title')),
      content: Text(s.t('rep2.discard.body')),
      actionsPadding: const EdgeInsets.fromLTRB(16, 0, 16, 16),
      actions: [
        OutlinedButton(
          style: OutlinedButton.styleFrom(foregroundColor: AppColors.danger),
          onPressed: () => Navigator.of(ctx).pop(true),
          child: Text(s.t('rep2.discard.yes')),
        ),
        FilledButton(onPressed: () => Navigator.of(ctx).pop(false), child: Text(s.t('rep2.discard.no'))),
      ],
    ),
  );
  return ok ?? false;
}

/// Confirmation after a report was queued; mentions the outbox when offline.
void showSaved(BuildContext context, String key) {
  final s = S.of(context);
  final online = context.read<Outbox>().online;
  showMessage(context, online ? s.t(key) : '${s.t(key)} ${s.t('att.queued')}');
}

/// Local wall-clock time as HH:mm.
String hhmm(DateTime t) {
  final l = t.toLocal();
  return '${l.hour.toString().padLeft(2, '0')}:${l.minute.toString().padLeft(2, '0')}';
}

/// Elapsed time as "45 mnt" / "2 j 15 mnt".
String duration(S s, Duration d) {
  final mins = d.inMinutes < 0 ? 0 : d.inMinutes;
  if (mins < 60) return s.t('common.minutes', {'n': mins});
  return s.t('rep2.dur.hm', {'h': mins ~/ 60, 'm': mins % 60});
}
