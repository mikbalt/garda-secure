import 'package:flutter/material.dart';

/// Design tokens shared with the web app (web/DESIGN.md): navy shell, brand
/// blue for actions, slate neutrals, emerald/amber/orange/red for status.
class AppColors {
  static const ink = Color(0xFF0D1420);
  static const ink800 = Color(0xFF141C2B);
  static const ink700 = Color(0xFF1C2638);

  static const primary = Color(0xFF2749E8);
  static const primary50 = Color(0xFFEEF4FF);
  static const primary100 = Color(0xFFDCE7FE);
  static const primary400 = Color(0xFF628EF8);
  static const primary700 = Color(0xFF1F38D4);

  static const bg = Color(0xFFF8FAFC);
  static const surface = Colors.white;
  static const border = Color(0xFFE2E8F0);
  static const text = Color(0xFF0F172A);
  static const textMuted = Color(0xFF64748B);
  static const textSubtle = Color(0xFF94A3B8);

  static const ok = Color(0xFF10B981);
  static const okBg = Color(0xFFECFDF5);
  static const warn = Color(0xFFF59E0B);
  static const warnBg = Color(0xFFFFFBEB);
  static const overdue = Color(0xFFF97316);
  static const overdueBg = Color(0xFFFFF7ED);
  static const danger = Color(0xFFDC2626);
  static const dangerBg = Color(0xFFFEF2F2);
  static const sos = Color(0xFFE11D48);
}

class AppTheme {
  static ThemeData light() {
    final scheme = ColorScheme.fromSeed(
      seedColor: AppColors.primary,
      primary: AppColors.primary,
      surface: AppColors.surface,
      error: AppColors.danger,
    );
    final base = ThemeData(useMaterial3: true, colorScheme: scheme, fontFamily: 'Inter', scaffoldBackgroundColor: AppColors.bg);
    final radius = BorderRadius.circular(14);
    return base.copyWith(
      textTheme: base.textTheme.apply(bodyColor: AppColors.text, displayColor: AppColors.text).copyWith(
            headlineMedium: const TextStyle(fontSize: 26, fontWeight: FontWeight.w600, letterSpacing: -0.4, color: AppColors.text),
            titleLarge: const TextStyle(fontSize: 20, fontWeight: FontWeight.w600, letterSpacing: -0.2, color: AppColors.text),
            titleMedium: const TextStyle(fontSize: 16, fontWeight: FontWeight.w600, color: AppColors.text),
            bodyMedium: const TextStyle(fontSize: 15, color: AppColors.text, height: 1.4),
            bodySmall: const TextStyle(fontSize: 13, color: AppColors.textMuted, height: 1.35),
            labelLarge: const TextStyle(fontSize: 15, fontWeight: FontWeight.w600),
          ).apply(fontFamily: 'Inter'),
      appBarTheme: const AppBarTheme(
        backgroundColor: AppColors.surface,
        foregroundColor: AppColors.text,
        elevation: 0,
        scrolledUnderElevation: 0.5,
        centerTitle: false,
        titleTextStyle: TextStyle(fontFamily: 'Inter', fontSize: 18, fontWeight: FontWeight.w600, color: AppColors.text),
      ),
      cardTheme: CardTheme(
        color: AppColors.surface,
        elevation: 0,
        margin: EdgeInsets.zero,
        shape: RoundedRectangleBorder(borderRadius: radius, side: const BorderSide(color: AppColors.border)),
      ),
      filledButtonTheme: FilledButtonThemeData(
        style: FilledButton.styleFrom(
          minimumSize: const Size(64, 52),
          padding: const EdgeInsets.symmetric(horizontal: 22),
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
          textStyle: const TextStyle(fontFamily: 'Inter', fontSize: 16, fontWeight: FontWeight.w600),
        ),
      ),
      outlinedButtonTheme: OutlinedButtonThemeData(
        style: OutlinedButton.styleFrom(
          minimumSize: const Size(64, 52),
          padding: const EdgeInsets.symmetric(horizontal: 20),
          foregroundColor: AppColors.text,
          side: const BorderSide(color: AppColors.border),
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
          textStyle: const TextStyle(fontFamily: 'Inter', fontSize: 16, fontWeight: FontWeight.w600),
        ),
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: AppColors.surface,
        contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 16),
        border: OutlineInputBorder(borderRadius: BorderRadius.circular(12), borderSide: const BorderSide(color: AppColors.border)),
        enabledBorder: OutlineInputBorder(borderRadius: BorderRadius.circular(12), borderSide: const BorderSide(color: Color(0xFFCBD5E1))),
        focusedBorder: OutlineInputBorder(borderRadius: BorderRadius.circular(12), borderSide: const BorderSide(color: AppColors.primary, width: 2)),
        labelStyle: const TextStyle(color: AppColors.textMuted),
      ),
      snackBarTheme: SnackBarThemeData(
        behavior: SnackBarBehavior.floating,
        backgroundColor: AppColors.ink,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
      ),
      dividerTheme: const DividerThemeData(color: AppColors.border, space: 1, thickness: 1),
      chipTheme: base.chipTheme.copyWith(shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(999))),
    );
  }
}
