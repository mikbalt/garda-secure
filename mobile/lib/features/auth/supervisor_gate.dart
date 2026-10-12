import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:provider/provider.dart';

import '../../core/auth/auth_provider.dart';
import '../../core/auth/keycloak_service.dart';
import '../../core/services/pin_verifier.dart';
import '../../l10n/strings.dart';
import '../../shared/models/models.dart';
import '../../shared/widgets/ui.dart';
import '../../theme/app_theme.dart';

/// Asks for a supervisor's approval before a protected action (MOB-2):
/// signing the device out, changing the server, discarding queued items.
///
/// Accepted: the PIN of a roster guard with role `supervisor` (checked
/// locally against the synced hash, so it works offline, with the same
/// persistent lockout as the lock screen), or the tablet's device account
/// password (checked online against Keycloak). Returns true when approved.
Future<bool> requireSupervisor(BuildContext context) async {
  final ok = await showDialog<bool>(context: context, builder: (_) => const SupervisorGateDialog());
  return ok == true;
}

class SupervisorGateDialog extends StatefulWidget {
  const SupervisorGateDialog({super.key});

  @override
  State<SupervisorGateDialog> createState() => _SupervisorGateDialogState();
}

class _SupervisorGateDialogState extends State<SupervisorGateDialog> {
  late final List<RosterGuard> _supervisors;
  RosterGuard? _sup;
  bool _password = false;
  bool _busy = false;
  String? _error;
  final _pin = TextEditingController();
  final _user = TextEditingController();
  final _pass = TextEditingController();

  @override
  void initState() {
    super.initState();
    final session = context.read<DeviceSession>();
    _supervisors = session.roster.where((g) => g.isSupervisor && g.hasPin).toList();
    _sup = _supervisors.length == 1 ? _supervisors.first : null;
    _password = _supervisors.isEmpty;
    session.auth.deviceUsername.then((u) {
      if (mounted && u != null && _user.text.isEmpty) _user.text = u;
    });
  }

  @override
  void dispose() {
    _pin.dispose();
    _user.dispose();
    _pass.dispose();
    super.dispose();
  }

  Future<void> _confirm() async {
    final s = S.of(context);
    final session = context.read<DeviceSession>();
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      bool ok;
      if (_password) {
        final cfg = session.config;
        ok = cfg != null && await session.auth.verifyDevicePassword(cfg, _user.text, _pass.text);
        if (!ok) _error = s.t('gate.wrong');
      } else {
        final sup = _sup;
        if (sup == null) return;
        final wait = session.pinAttempts.waitSeconds(sup.id);
        if (wait > 0) {
          _error = s.t('lock.tooMany', {'s': wait});
          return;
        }
        ok = await PinVerifier.verify(_pin.text, sup.pinHash!);
        if (ok) {
          session.pinAttempts.success(sup.id);
        } else {
          final left = session.pinAttempts.fail(sup.id);
          _error = left > 0 ? s.t('lock.wrongPin', {'n': left}) : s.t('lock.tooMany', {'s': session.pinAttempts.waitSeconds(sup.id)});
          _pin.clear();
        }
      }
      if (ok && mounted) Navigator.of(context).pop(true);
    } on LoginFailure catch (e) {
      _error = e.reason == 'unreachable' ? s.t('gate.offline') : s.t('gate.wrong');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final canSubmit = !_busy && (_password ? _user.text.trim().isNotEmpty && _pass.text.isNotEmpty : _sup != null && _pin.text.length >= 4);
    return AlertDialog(
      title: Row(children: [
        const IconTile(icon: Icons.admin_panel_settings_outlined, color: AppColors.primary, size: 40),
        const SizedBox(width: 12),
        Expanded(child: Text(s.t('gate.title'))),
      ]),
      content: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 420, minWidth: 320),
        child: SingleChildScrollView(
          child: Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
            Text(s.t('gate.subtitle'), style: const TextStyle(color: AppColors.textMuted, height: 1.4)),
            const SizedBox(height: 18),
            if (!_password) ...[
              FieldLabel(s.t('gate.pickSupervisor')),
              Wrap(spacing: 8, runSpacing: 8, children: [
                for (final g in _supervisors)
                  ChoiceChip(
                    label: Text(g.name),
                    selected: _sup?.id == g.id,
                    onSelected: (_) => setState(() {
                      _sup = g;
                      _error = null;
                    }),
                  ),
              ]),
              const SizedBox(height: 14),
              TextField(
                key: const ValueKey('gate-pin'),
                controller: _pin,
                obscureText: true,
                keyboardType: TextInputType.number,
                maxLength: 6,
                inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                decoration: const InputDecoration(labelText: 'PIN', prefixIcon: Icon(Icons.pin_outlined), counterText: ''),
                onChanged: (_) => setState(() {}),
                onSubmitted: (_) => canSubmit ? _confirm() : null,
              ),
            ] else ...[
              if (_supervisors.isEmpty) ...[
                Text(s.t('gate.noSupervisor'), style: const TextStyle(color: AppColors.textMuted, fontSize: 13)),
                const SizedBox(height: 12),
              ],
              TextField(
                key: const ValueKey('gate-user'),
                controller: _user,
                autocorrect: false,
                decoration: InputDecoration(labelText: s.t('gate.username'), prefixIcon: const Icon(Icons.person_outline)),
                onChanged: (_) => setState(() {}),
              ),
              const SizedBox(height: 12),
              TextField(
                key: const ValueKey('gate-pass'),
                controller: _pass,
                obscureText: true,
                decoration: InputDecoration(labelText: s.t('gate.password'), prefixIcon: const Icon(Icons.lock_outline)),
                onChanged: (_) => setState(() {}),
                onSubmitted: (_) => canSubmit ? _confirm() : null,
              ),
            ],
            if (_error != null) ...[
              const SizedBox(height: 12),
              Text(_error!, style: const TextStyle(color: AppColors.danger, fontWeight: FontWeight.w500)),
            ],
            if (_supervisors.isNotEmpty) ...[
              const SizedBox(height: 8),
              Align(
                alignment: Alignment.centerLeft,
                child: TextButton(
                  onPressed: _busy
                      ? null
                      : () => setState(() {
                            _password = !_password;
                            _error = null;
                          }),
                  child: Text(_password ? s.t('gate.useSupervisor') : s.t('gate.usePassword')),
                ),
              ),
            ],
          ]),
        ),
      ),
      actions: [
        TextButton(onPressed: () => Navigator.of(context).pop(false), child: Text(s.t('common.cancel'))),
        FilledButton(
          onPressed: canSubmit ? _confirm : null,
          child: _busy ? const SizedBox(width: 18, height: 18, child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white)) : Text(s.t('gate.confirm')),
        ),
      ],
    );
  }
}
