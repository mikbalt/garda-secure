import 'package:flutter/material.dart';
import '../../shared/widgets/sos_button.dart';
import '../../shared/widgets/sync_indicator.dart';

class HomeScreen extends StatelessWidget {
  const HomeScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('SecurePatrol'),
        backgroundColor: const Color(0xFF1e40af),
        foregroundColor: Colors.white,
        actions: const [SyncIndicator()],
      ),
      body: const Center(
        child: Text('Home — coming in M2', style: TextStyle(color: Colors.grey)),
      ),
      floatingActionButton: const SosButton(),
    );
  }
}
