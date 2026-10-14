import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:provider/provider.dart';

import 'core/auth/auth_provider.dart';
import 'core/database/sync_service.dart';
import 'features/auth/pin_screen.dart';
import 'features/field_actions.dart';
import 'features/home/home_screen.dart';
import 'features/setup/pending_screen.dart';
import 'features/setup/setup_screen.dart';
import 'features/sos/sos_alerts_screen.dart';
import 'shared/widgets/brand.dart';
import 'shared/widgets/sos_button.dart';
import 'theme/app_theme.dart';

final rootNavigator = GlobalKey<NavigatorState>();

/// Opens what a tapped notification points at: `sos:<id>` → the SOS alerts
/// (only for an unlocked tablet — the lock screen shows the SOS strip and
/// details need a PIN), `home` → back to the home screen.
void openPushTarget(DeviceSession session, String payload) {
  WidgetsBinding.instance.addPostFrameCallback((_) {
    final nav = rootNavigator.currentState;
    if (nav == null || session.phase != Phase.unlocked) {
      if (payload.startsWith('sos')) unawaited(session.refreshSos());
      return;
    }
    if (payload.startsWith('sos')) {
      unawaited(session.refreshSos());
      String? top;
      nav.popUntil((r) {
        top = r.settings.name;
        return true;
      });
      if (top != SosAlertsScreen.routeName) {
        unawaited(nav.push(MaterialPageRoute<void>(settings: const RouteSettings(name: SosAlertsScreen.routeName), builder: (_) => const SosAlertsScreen())));
      }
    } else {
      nav.popUntil((r) => r.isFirst);
    }
  });
  WidgetsBinding.instance.scheduleFrame();
}

class SecurePatrolApp extends StatelessWidget {
  const SecurePatrolApp({super.key});

  @override
  Widget build(BuildContext context) {
    final session = context.watch<DeviceSession>();
    return Listener(
      // Any touch counts as activity for the idle auto-lock (FR-AUTH-04).
      onPointerDown: (_) => session.touch(),
      behavior: HitTestBehavior.translucent,
      child: MaterialApp(
        title: 'SecurePatrol',
        navigatorKey: rootNavigator,
        debugShowCheckedModeBanner: false,
        theme: AppTheme.light(),
        locale: session.locale,
        supportedLocales: const [Locale('id'), Locale('en')],
        localizationsDelegates: GlobalMaterialLocalizations.delegates,
        builder: (context, child) => Column(
          children: [
            const SosDeliveryBanner(),
            Expanded(child: child ?? const SizedBox.shrink()),
          ],
        ),
        home: const _Gate(),
      ),
    );
  }
}

/// Shows the screen for the tablet's phase and keeps guard state in sync.
class _Gate extends StatefulWidget {
  const _Gate();

  @override
  State<_Gate> createState() => _GateState();
}

class _GateState extends State<_Gate> {
  Phase? _lastPhase;
  String? _lastGuard;
  GuardStatusRef? _lastStatus;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    final session = context.read<DeviceSession>();
    final outbox = context.read<Outbox>();
    session.removeListener(_onSession);
    session.addListener(_onSession);
    outbox.removeListener(_onOutbox);
    outbox.addListener(_onOutbox);
    _onSession();
  }

  void _onSession() {
    if (!mounted) return;
    final session = context.read<DeviceSession>();
    final actions = context.read<FieldActions>();
    final guardId = session.guard?.id;
    if (session.phase != _lastPhase || guardId != _lastGuard) {
      // Locking (manually or after idle) closes every open screen so the next
      // guard never sees the previous one's work.
      if (_lastPhase == Phase.unlocked && session.phase != Phase.unlocked) {
        rootNavigator.currentState?.popUntil((r) => r.isFirst);
        actions.clear();
      }
      if (session.phase == Phase.unlocked && guardId != null) unawaited(actions.loadForGuard());
      _lastPhase = session.phase;
      _lastGuard = guardId;
    }
    final status = GuardStatusRef(session.guardStatus);
    if (status != _lastStatus) {
      _lastStatus = status;
      if (session.guardStatus != null) unawaited(actions.reconcile());
    }
  }

  void _onOutbox() {
    if (!mounted) return;
    unawaited(context.read<FieldActions>().refreshSosDelivery());
  }

  @override
  void dispose() {
    context.read<DeviceSession>().removeListener(_onSession);
    context.read<Outbox>().removeListener(_onOutbox);
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final phase = context.select<DeviceSession, Phase>((s) => s.phase);
    return AnimatedSwitcher(
      duration: const Duration(milliseconds: 220),
      child: switch (phase) {
        Phase.booting => const _Splash(),
        Phase.setup => const ServerSetupScreen(),
        Phase.login => const DeviceLoginScreen(),
        Phase.pending || Phase.revoked => const PendingScreen(),
        Phase.locked => const PinScreen(),
        Phase.unlocked => const HomeScreen(),
      },
    );
  }
}

/// Identity wrapper so a new status object is detected even if equal-looking.
class GuardStatusRef {
  GuardStatusRef(this.value);
  final Object? value;
  @override
  bool operator ==(Object other) => other is GuardStatusRef && identical(other.value, value);
  @override
  int get hashCode => identityHashCode(value);
}

class _Splash extends StatelessWidget {
  const _Splash();

  @override
  Widget build(BuildContext context) => const Scaffold(
        backgroundColor: AppColors.ink,
        body: Center(child: BrandMark(size: 64)),
      );
}
