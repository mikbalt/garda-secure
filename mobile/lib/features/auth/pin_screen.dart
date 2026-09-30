import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../../core/auth/auth_provider.dart';
import '../../shared/widgets/sync_indicator.dart';

class PinScreen extends StatefulWidget {
  const PinScreen({super.key});

  @override
  State<PinScreen> createState() => _PinScreenState();
}

class _PinScreenState extends State<PinScreen> {
  final _guardIdController = TextEditingController();
  String _pin = '';
  bool _loading = false;
  String? _error;

  void _onDigit(String d) {
    if (_pin.length < 6) setState(() => _pin += d);
    if (_pin.length == 6) _submit();
  }

  void _onBackspace() {
    if (_pin.isNotEmpty) setState(() => _pin = _pin.substring(0, _pin.length - 1));
  }

  Future<void> _submit() async {
    final guardId = _guardIdController.text.trim();
    if (guardId.isEmpty) {
      setState(() => _error = 'Enter your Guard ID first');
      _pin = '';
      return;
    }
    setState(() { _loading = true; _error = null; });
    try {
      await context.read<AuthProvider>().loginWithPin(guardId, _pin);
      if (mounted) Navigator.pushReplacementNamed(context, '/home');
    } catch (e) {
      setState(() { _error = e.toString(); _pin = ''; });
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('SecurePatrol'),
        backgroundColor: const Color(0xFF1e40af),
        foregroundColor: Colors.white,
        actions: const [SyncIndicator()],
      ),
      body: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.symmetric(horizontal: 32, vertical: 24),
          child: Column(
            children: [
              const SizedBox(height: 16),
              // Guard ID field
              TextField(
                controller: _guardIdController,
                decoration: const InputDecoration(
                  labelText: 'Guard ID',
                  border: OutlineInputBorder(),
                  prefixIcon: Icon(Icons.badge),
                ),
                keyboardType: TextInputType.text,
                textInputAction: TextInputAction.next,
              ),
              const SizedBox(height: 32),
              // PIN dots
              Row(
                mainAxisAlignment: MainAxisAlignment.center,
                children: List.generate(6, (i) => Container(
                  margin: const EdgeInsets.symmetric(horizontal: 8),
                  width: 18,
                  height: 18,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    color: i < _pin.length
                        ? const Color(0xFF1e40af)
                        : Colors.grey.shade300,
                    border: Border.all(color: const Color(0xFF1e40af), width: 1.5),
                  ),
                )),
              ),
              if (_error != null) ...[
                const SizedBox(height: 16),
                Text(_error!, style: const TextStyle(color: Colors.red)),
              ],
              const SizedBox(height: 32),
              // Number pad
              if (_loading)
                const CircularProgressIndicator()
              else
                _buildNumberPad(),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildNumberPad() {
    final rows = [
      ['1', '2', '3'],
      ['4', '5', '6'],
      ['7', '8', '9'],
      ['', '0', '⌫'],
    ];
    return Column(
      children: rows.map((row) => Row(
        mainAxisAlignment: MainAxisAlignment.center,
        children: row.map((label) {
          if (label.isEmpty) return const SizedBox(width: 80, height: 64);
          return Padding(
            padding: const EdgeInsets.all(6),
            child: SizedBox(
              width: 80,
              height: 64,
              child: ElevatedButton(
                style: ElevatedButton.styleFrom(
                  backgroundColor: label == '⌫'
                      ? Colors.grey.shade200
                      : const Color(0xFF1e40af),
                  foregroundColor: label == '⌫' ? Colors.black87 : Colors.white,
                  shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
                ),
                onPressed: label == '⌫' ? _onBackspace : () => _onDigit(label),
                child: Text(label, style: const TextStyle(fontSize: 22, fontWeight: FontWeight.w500)),
              ),
            ),
          );
        }).toList(),
      )).toList(),
    );
  }
}
