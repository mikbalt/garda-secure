// Screenshots of the onboarding and lock screens. Not part of `flutter test`:
//   flutter test screenshots/core_shots_test.dart --update-goldens
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:securepatrol/core/auth/auth_provider.dart';
import 'package:securepatrol/features/auth/pin_screen.dart';
import 'package:securepatrol/features/setup/pending_screen.dart';
import 'package:securepatrol/features/setup/setup_screen.dart';
import 'package:securepatrol/shared/widgets/brand.dart';

import '../test/support/harness.dart';

void main() {
  late Harness h;
  setUpAll(() async {
    await loadAppFonts();
    h = await Harness.create();
    h.seedSite(unlocked: false);
  });

  Future<void> shot(WidgetTester tester, Widget w, String name, {Size size = tabletSize, Future<void> Function()? act, bool settle = true}) async {
    await setSurface(tester, size);
    await tester.pumpWidget(wrap(h, w));
    // Decode the brand mark (an asset image) outside the fake clock.
    final ctx = tester.element(find.byType(MaterialApp));
    await tester.runAsync(() => precacheImage(const AssetImage(BrandMark.asset), ctx));
    settle ? await tester.pumpAndSettle(const Duration(milliseconds: 300)) : await tester.pump(const Duration(milliseconds: 500));
    if (act != null) {
      await act();
      await tester.pumpAndSettle(const Duration(milliseconds: 300));
    }
    await expectLater(find.byType(MaterialApp), matchesGoldenFile('out/core_$name.png'));
    await tester.pumpWidget(const SizedBox()); // stop timers
  }

  testWidgets('server setup', (t) => shot(t, const ServerSetupScreen(), 'setup'));
  testWidgets('server setup phone', (t) => shot(t, const ServerSetupScreen(), 'setup_phone', size: phoneSize));
  testWidgets('device login', (t) => shot(t, const DeviceLoginScreen(), 'login'));
  testWidgets('pending approval', (t) async {
    h.session.phase = Phase.pending;
    await shot(t, const PendingScreen(), 'pending', settle: false);
    h.session.phase = Phase.locked;
  });
  testWidgets('lock screen', (t) => shot(t, const PinScreen(), 'lock'));
  testWidgets('lock screen with guard selected', (t) => shot(t, const PinScreen(), 'lock_pin', act: () async {
        await t.tap(find.text('Budi Santoso'));
        await t.pump();
        await t.tap(find.text('4'));
        await t.tap(find.text('8'));
      }));
  testWidgets('lock screen phone', (t) => shot(t, const PinScreen(), 'lock_phone', size: phoneSize));
}
