import 'dart:io';

import 'package:flutter/material.dart';

import '../../theme/app_theme.dart';

/// White card with the app's border and radius.
class SectionCard extends StatelessWidget {
  const SectionCard({super.key, required this.child, this.padding = const EdgeInsets.all(18), this.color, this.borderColor, this.onTap});

  final Widget child;
  final EdgeInsets padding;
  final Color? color;
  final Color? borderColor;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final body = Padding(padding: padding, child: child);
    return Material(
      color: color ?? AppColors.surface,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16), side: BorderSide(color: borderColor ?? AppColors.border)),
      clipBehavior: Clip.antiAlias,
      child: onTap == null ? body : InkWell(onTap: onTap, child: body),
    );
  }
}

/// Tinted rounded square holding an icon.
class IconTile extends StatelessWidget {
  const IconTile({super.key, required this.icon, required this.color, this.size = 44});

  final IconData icon;
  final Color color;
  final double size;

  @override
  Widget build(BuildContext context) => Container(
        width: size,
        height: size,
        decoration: BoxDecoration(color: color.withOpacity(0.1), borderRadius: BorderRadius.circular(size * 0.3)),
        child: Icon(icon, color: color, size: size * 0.5),
      );
}

/// Big touch target on the home screen.
class ActionTile extends StatelessWidget {
  const ActionTile({super.key, required this.icon, required this.title, required this.subtitle, required this.color, required this.onTap, this.badge, this.highlight = false});

  final IconData icon;
  final String title;
  final String subtitle;
  final Color color;
  final VoidCallback onTap;
  final String? badge;
  final bool highlight;

  @override
  Widget build(BuildContext context) {
    return SectionCard(
      onTap: onTap,
      borderColor: highlight ? color.withOpacity(0.5) : null,
      color: highlight ? color.withOpacity(0.04) : null,
      child: Row(
        children: [
          IconTile(icon: icon, color: color, size: 52),
          const SizedBox(width: 16),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(title, style: Theme.of(context).textTheme.titleMedium),
                const SizedBox(height: 2),
                Text(subtitle, style: Theme.of(context).textTheme.bodySmall, maxLines: 2, overflow: TextOverflow.ellipsis),
              ],
            ),
          ),
          if (badge != null) ...[
            const SizedBox(width: 8),
            Pill(text: badge!, color: color),
          ],
          const SizedBox(width: 4),
          const Icon(Icons.chevron_right_rounded, color: AppColors.textSubtle),
        ],
      ),
    );
  }
}

/// Small rounded status label.
class Pill extends StatelessWidget {
  const Pill({super.key, required this.text, required this.color, this.icon, this.dot = false});

  final String text;
  final Color color;
  final IconData? icon;
  final bool dot;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
      decoration: BoxDecoration(color: color.withOpacity(0.1), borderRadius: BorderRadius.circular(999), border: Border.all(color: color.withOpacity(0.25))),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          if (dot) ...[
            Container(width: 7, height: 7, decoration: BoxDecoration(color: color, shape: BoxShape.circle)),
            const SizedBox(width: 6),
          ],
          if (icon != null) ...[
            Icon(icon, size: 14, color: color),
            const SizedBox(width: 4),
          ],
          Text(text, style: TextStyle(fontSize: 12.5, fontWeight: FontWeight.w600, color: Color.lerp(color, Colors.black, 0.25))),
        ],
      ),
    );
  }
}

/// Initials avatar.
class Avatar extends StatelessWidget {
  const Avatar({super.key, required this.initials, this.size = 48, this.color = AppColors.primary});

  final String initials;
  final double size;
  final Color color;

  @override
  Widget build(BuildContext context) => Container(
        width: size,
        height: size,
        alignment: Alignment.center,
        decoration: BoxDecoration(color: color.withOpacity(0.12), shape: BoxShape.circle),
        child: Text(initials, style: TextStyle(fontSize: size * 0.36, fontWeight: FontWeight.w600, color: color)),
      );
}

/// Page header used at the top of form screens.
class ScreenIntro extends StatelessWidget {
  const ScreenIntro({super.key, required this.icon, required this.color, required this.title, this.subtitle});

  final IconData icon;
  final Color color;
  final String title;
  final String? subtitle;

  @override
  Widget build(BuildContext context) {
    final tt = Theme.of(context).textTheme;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        IconTile(icon: icon, color: color, size: 48),
        const SizedBox(width: 14),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(title, style: tt.titleLarge),
              if (subtitle != null) ...[const SizedBox(height: 4), Text(subtitle!, style: tt.bodySmall)],
            ],
          ),
        ),
      ],
    );
  }
}

/// Centered constrained column so tablet layouts don't stretch edge to edge.
class Constrained extends StatelessWidget {
  const Constrained({super.key, required this.child, this.maxWidth = 760});

  final Widget child;
  final double maxWidth;

  @override
  Widget build(BuildContext context) => Center(child: ConstrainedBox(constraints: BoxConstraints(maxWidth: maxWidth), child: child));
}

/// Square photo thumbnail with remove / add states.
class PhotoThumb extends StatelessWidget {
  const PhotoThumb({super.key, this.path, this.onTap, this.onRemove, this.label, this.size = 96});

  final String? path;
  final VoidCallback? onTap;
  final VoidCallback? onRemove;
  final String? label;
  final double size;

  @override
  Widget build(BuildContext context) {
    final has = path != null;
    return SizedBox(
      width: size,
      height: size,
      child: Stack(
        children: [
          Positioned.fill(
            child: Material(
              color: has ? Colors.black : AppColors.primary50,
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(14),
                side: BorderSide(color: has ? AppColors.border : AppColors.primary100, width: has ? 1 : 1.5),
              ),
              clipBehavior: Clip.antiAlias,
              child: InkWell(
                onTap: onTap,
                child: has
                    ? Image.file(File(path!), fit: BoxFit.cover, errorBuilder: (_, __, ___) => const Icon(Icons.broken_image_outlined))
                    : Column(
                        mainAxisAlignment: MainAxisAlignment.center,
                        children: [
                          const Icon(Icons.photo_camera_outlined, color: AppColors.primary, size: 28),
                          if (label != null) ...[
                            const SizedBox(height: 4),
                            Padding(
                              padding: const EdgeInsets.symmetric(horizontal: 6),
                              child: Text(label!, textAlign: TextAlign.center, style: const TextStyle(fontSize: 12, color: AppColors.primary700, fontWeight: FontWeight.w600)),
                            ),
                          ],
                        ],
                      ),
              ),
            ),
          ),
          if (has && onRemove != null)
            Positioned(
              top: 4,
              right: 4,
              child: Material(
                color: Colors.black54,
                shape: const CircleBorder(),
                child: InkWell(customBorder: const CircleBorder(), onTap: onRemove, child: const Padding(padding: EdgeInsets.all(4), child: Icon(Icons.close, size: 16, color: Colors.white))),
              ),
            ),
        ],
      ),
    );
  }
}

/// Choice chips for a small set of options.
class ChoiceRow<T> extends StatelessWidget {
  const ChoiceRow({super.key, required this.options, required this.value, required this.onChanged, required this.label, this.colorOf});

  final List<T> options;
  final T? value;
  final ValueChanged<T> onChanged;
  final String Function(T) label;
  final Color Function(T)? colorOf;

  @override
  Widget build(BuildContext context) {
    return Wrap(
      spacing: 8,
      runSpacing: 8,
      children: [
        for (final o in options)
          ChoiceChip(
            label: Text(label(o)),
            selected: o == value,
            onSelected: (_) => onChanged(o),
            labelStyle: TextStyle(fontWeight: FontWeight.w600, color: o == value ? Colors.white : AppColors.text),
            selectedColor: colorOf?.call(o) ?? AppColors.primary,
            backgroundColor: AppColors.surface,
            side: BorderSide(color: o == value ? Colors.transparent : AppColors.border),
            showCheckmark: false,
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 10),
          ),
      ],
    );
  }
}

/// Field label above a control.
class FieldLabel extends StatelessWidget {
  const FieldLabel(this.text, {super.key, this.optional});

  final String text;
  final String? optional;

  @override
  Widget build(BuildContext context) => Padding(
        padding: const EdgeInsets.only(bottom: 8),
        child: Text.rich(TextSpan(children: [
          TextSpan(text: text, style: const TextStyle(fontSize: 14, fontWeight: FontWeight.w600, color: AppColors.text)),
          if (optional != null) TextSpan(text: '  ($optional)', style: const TextStyle(fontSize: 13, color: AppColors.textSubtle)),
        ])),
      );
}

void showMessage(BuildContext context, String text, {bool error = false}) {
  ScaffoldMessenger.of(context)
    ..hideCurrentSnackBar()
    ..showSnackBar(SnackBar(
      content: Row(children: [
        Icon(error ? Icons.error_outline : Icons.check_circle_outline, color: error ? const Color(0xFFFCA5A5) : const Color(0xFF6EE7B7)),
        const SizedBox(width: 10),
        Expanded(child: Text(text)),
      ]),
    ));
}
