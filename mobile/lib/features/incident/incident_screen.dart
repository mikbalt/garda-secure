import 'package:flutter/material.dart';
import '../../shared/widgets/sos_button.dart';
import '../../shared/widgets/sync_indicator.dart';

class IncidentScreen extends StatelessWidget {
  const IncidentScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Incidents'),
        backgroundColor: const Color(0xFF1e40af),
        foregroundColor: Colors.white,
        actions: const [SyncIndicator()],
      ),
      body: const Center(
        child: Text('Incident reporting — coming in M4', style: TextStyle(color: Colors.grey)),
      ),
      floatingActionButton: const SosButton(),
    );
  }
}
