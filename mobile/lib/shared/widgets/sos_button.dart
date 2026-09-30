import 'dart:async';
import 'package:flutter/material.dart';
import 'package:geolocator/geolocator.dart';
import 'package:provider/provider.dart';
import 'package:connectivity_plus/connectivity_plus.dart';
import '../../../core/api/api_client.dart';

/// Persistent SOS panic button — posts to /api/v1/incidents/sos with GPS.
/// If offline, queues locally and retries every 10 seconds.
class SosButton extends StatefulWidget {
  const SosButton({super.key});

  @override
  State<SosButton> createState() => _SosButtonState();
}

class _SosButtonState extends State<SosButton> {
  Timer? _retryTimer;
  bool _pending = false;

  @override
  void dispose() {
    _retryTimer?.cancel();
    super.dispose();
  }

  Future<void> _send() async {
    Position? pos;
    try {
      pos = await Geolocator.getCurrentPosition(
          desiredAccuracy: LocationAccuracy.high);
    } catch (_) {}

    final api = context.read<ApiClient>();
    try {
      await api.dio.post('/api/v1/incidents/sos', data: {
        'gps_lat': pos?.latitude,
        'gps_lng': pos?.longitude,
      });
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('SOS sent — supervisor alerted'), backgroundColor: Colors.red),
        );
      }
      _retryTimer?.cancel();
      setState(() => _pending = false);
    } catch (_) {
      // Offline: queue and retry
      setState(() => _pending = true);
      _retryTimer?.cancel();
      _retryTimer = Timer.periodic(const Duration(seconds: 10), (_) => _retryQueued(pos));
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(
            content: Text('SOS queued — will send when online'),
            backgroundColor: Colors.orange,
          ),
        );
      }
    }
  }

  Future<void> _retryQueued(Position? pos) async {
    final connectivity = await Connectivity().checkConnectivity();
    if (connectivity == ConnectivityResult.none) return;
    final api = context.read<ApiClient>();
    try {
      await api.dio.post('/api/v1/incidents/sos', data: {
        'gps_lat': pos?.latitude,
        'gps_lng': pos?.longitude,
      });
      _retryTimer?.cancel();
      if (mounted) {
        setState(() => _pending = false);
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('SOS delivered'), backgroundColor: Colors.green),
        );
      }
    } catch (_) {}
  }

  Future<void> _onTap() async {
    final confirm = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Send SOS Alert?'),
        content: const Text('This will immediately notify all supervisors at your site.'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('Cancel')),
          FilledButton(
            style: FilledButton.styleFrom(backgroundColor: Colors.red),
            onPressed: () => Navigator.pop(context, true),
            child: const Text('Send SOS'),
          ),
        ],
      ),
    );
    if (confirm == true) await _send();
  }

  @override
  Widget build(BuildContext context) {
    return FloatingActionButton.extended(
      onPressed: _onTap,
      backgroundColor: Colors.red,
      foregroundColor: Colors.white,
      icon: _pending
          ? const SizedBox(width: 18, height: 18,
              child: CircularProgressIndicator(color: Colors.white, strokeWidth: 2))
          : const Icon(Icons.sos),
      label: Text(_pending ? 'SOS Queued' : 'SOS'),
    );
  }
}
