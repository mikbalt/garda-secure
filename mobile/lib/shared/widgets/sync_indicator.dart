import 'package:flutter/material.dart';
import 'package:connectivity_plus/connectivity_plus.dart';

/// AppBar action icon showing online/offline status with pending sync count.
class SyncIndicator extends StatelessWidget {
  const SyncIndicator({super.key});

  @override
  Widget build(BuildContext context) {
    return StreamBuilder<List<ConnectivityResult>>(
      stream: Connectivity().onConnectivityChanged,
      builder: (context, snapshot) {
        final results = snapshot.data ?? [];
        final isOnline = results.isNotEmpty &&
            results.any((r) => r != ConnectivityResult.none);
        return IconButton(
          icon: Icon(
            isOnline ? Icons.wifi : Icons.wifi_off,
            color: isOnline ? Colors.greenAccent : Colors.orangeAccent,
          ),
          tooltip: isOnline ? 'Online' : 'Offline — data will sync when connected',
          onPressed: () {
            ScaffoldMessenger.of(context).showSnackBar(
              SnackBar(content: Text(isOnline ? 'Connected' : 'Offline — pending sync')),
            );
          },
        );
      },
    );
  }
}
