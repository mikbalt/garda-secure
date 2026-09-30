import 'package:flutter/material.dart';
import '../../shared/widgets/sos_button.dart';
import '../../shared/widgets/sync_indicator.dart';

class PatrolScreen extends StatelessWidget {
  const PatrolScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Patrol'),
        backgroundColor: const Color(0xFF1e40af),
        foregroundColor: Colors.white,
        actions: const [SyncIndicator()],
      ),
      body: const Center(
        child: Text('QR checkpoint scanner — coming in M3', style: TextStyle(color: Colors.grey)),
      ),
      floatingActionButton: const SosButton(),
    );
  }
}
