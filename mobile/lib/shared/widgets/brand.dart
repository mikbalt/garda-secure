import 'package:flutter/material.dart';

import '../../theme/app_theme.dart';

/// App logo: shield in a brand-blue rounded square (same as the web app).
class BrandMark extends StatelessWidget {
  const BrandMark({super.key, this.size = 44});

  final double size;

  @override
  Widget build(BuildContext context) => Container(
        width: size,
        height: size,
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(size * 0.3),
          gradient: const LinearGradient(colors: [AppColors.primary400, AppColors.primary700], begin: Alignment.topLeft, end: Alignment.bottomRight),
          boxShadow: [BoxShadow(color: AppColors.primary.withOpacity(0.35), blurRadius: 16, offset: const Offset(0, 6))],
        ),
        child: Icon(Icons.verified_user_rounded, color: Colors.white, size: size * 0.55),
      );
}

/// Dark navy panel with brand glow, used behind setup and lock screens.
class BrandBackdrop extends StatelessWidget {
  const BrandBackdrop({super.key, required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) => Container(
        decoration: const BoxDecoration(
          color: AppColors.ink,
          gradient: RadialGradient(center: Alignment(-0.9, -1), radius: 1.4, colors: [Color(0xFF1E2E6B), AppColors.ink]),
        ),
        child: child,
      );
}
