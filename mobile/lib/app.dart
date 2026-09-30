import 'package:flutter/material.dart';
import 'features/auth/pin_screen.dart';
import 'features/attendance/clock_in_screen.dart';
import 'features/patrol/patrol_screen.dart';
import 'features/incident/incident_screen.dart';

class SecurePatrolApp extends StatelessWidget {
  const SecurePatrolApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'SecurePatrol',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        colorScheme: ColorScheme.fromSeed(
          seedColor: const Color(0xFF1e40af),
          brightness: Brightness.light,
        ),
        useMaterial3: true,
        fontFamily: 'Roboto',
      ),
      initialRoute: '/pin',
      routes: {
        '/pin':      (_) => const PinScreen(),
        '/home':     (_) => const HomeScreen(),
        '/patrol':   (_) => const PatrolScreen(),
        '/incident': (_) => const IncidentScreen(),
      },
    );
  }
}
