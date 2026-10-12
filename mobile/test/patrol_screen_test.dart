import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:securepatrol/features/patrol/patrol_screen.dart';

import 'support/harness.dart';

void main() {
  testWidgets('pick a route, scan every checkpoint, finish', (tester) async {
    late Harness h;
    await tester.runAsync(() async {
      h = await Harness.create();
      h.seedSite();
    });
    await setSurface(tester, tabletSize);
    await tester.pumpWidget(wrap(h, const PatrolScreen()));
    await tester.pump();

    expect(find.text('Patroli Keliling Gudang'), findsOneWidget);
    await tester.tap(find.text('Mulai patroli').first);
    await tester.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 200)));
    await tester.pump();
    expect(h.actions.patrol, isNotNull);
    expect(find.text('Pindai QR'), findsOneWidget);
    expect(find.text('Gerbang Utama'), findsWidgets);

    await tester.runAsync(() async {
      expect(await h.actions.scan('NOT-ON-ROUTE'), isNull);
      for (final cp in h.session.routes.first.checkpoints) {
        expect(await h.actions.scan(cp.qrCode), isNotNull);
      }
    });
    await tester.pump();
    expect(find.text('Semua titik sudah dipindai'), findsOneWidget);

    await tester.tap(find.text('Selesaikan patroli').last);
    await tester.pump();
    await tester.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 200)));
    await tester.pump();
    expect(h.actions.patrol, isNull);
    expect(find.text('Patroli selesai. Terima kasih!'), findsOneWidget);
    expect((await tester.runAsync(h.outbox.list))!.map((o) => o.kind), containsAllInOrder(['session_start', 'scan', 'session_end']));

    await tester.pumpWidget(const SizedBox());
    await tester.runAsync(h.dispose);
  });
}
