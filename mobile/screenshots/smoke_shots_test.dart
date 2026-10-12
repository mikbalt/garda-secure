// Screenshots (not part of `flutter test`): run
//   flutter test screenshots/ --update-goldens
// and look at screenshots/out/*.png.
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:securepatrol/shared/widgets/sos_button.dart';
import 'package:securepatrol/shared/widgets/ui.dart';
import 'package:securepatrol/theme/app_theme.dart';

import '../test/support/harness.dart';

void main() {
  late Harness h;
  setUpAll(() async {
    await loadAppFonts();
    h = await Harness.create();
    h.seedSite();
  });

  testWidgets('smoke', (tester) async {
    await setSurface(tester, const Size(600, 400));
    await tester.pumpWidget(wrap(
      h,
      const Scaffold(
        body: Padding(
          padding: EdgeInsets.all(24),
          child: Column(children: [
            ActionTile(icon: Icons.route, title: 'Patroli', subtitle: 'Pindai titik patroli', color: AppColors.primary, onTap: _noop, badge: '2'),
            SizedBox(height: 24),
            SosButton(),
          ]),
        ),
      ),
    ));
    await expectLater(find.byType(MaterialApp), matchesGoldenFile('out/smoke.png'));
  });
}

void _noop() {}
