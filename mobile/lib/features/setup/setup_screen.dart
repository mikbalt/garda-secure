import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../../config/env.dart';
import '../../core/api/api_client.dart';
import '../../core/api/tls.dart';
import '../../core/auth/auth_provider.dart';
import '../../core/auth/keycloak_service.dart';
import '../../l10n/strings.dart';
import '../../shared/widgets/brand.dart';
import '../../theme/app_theme.dart';
import '../auth/supervisor_gate.dart';
import '../device/device_info_screen.dart';

/// Split layout of the onboarding screens: brand panel with the three
/// onboarding steps on tablets, the form alone on phones.
class OnboardingFrame extends StatelessWidget {
  const OnboardingFrame({super.key, required this.step, required this.child});

  /// 0 = server, 1 = sign in, 2 = approval.
  final int step;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final wide = MediaQuery.sizeOf(context).width >= 900;
    final form = SafeArea(
      child: Center(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(28),
          child: ConstrainedBox(constraints: const BoxConstraints(maxWidth: 460), child: child),
        ),
      ),
    );
    if (!wide) return Scaffold(backgroundColor: AppColors.surface, body: form);
    final steps = [s.t('setup.title'), s.t('login.title'), s.t('pending.title')];
    return Scaffold(
      backgroundColor: AppColors.surface,
      body: Row(
        children: [
          Expanded(
            flex: 5,
            child: BrandBackdrop(
              child: SafeArea(
                child: Padding(
                  padding: const EdgeInsets.all(44),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      const Row(children: [
                        BrandMark(),
                        SizedBox(width: 14),
                        Text('SecurePatrol', style: TextStyle(color: Colors.white, fontSize: 20, fontWeight: FontWeight.w600)),
                      ]),
                      const Spacer(),
                      Text(
                        s.lang == 'en' ? 'Post tablet setup' : 'Pengaturan tablet pos jaga',
                        style: const TextStyle(color: Colors.white, fontSize: 30, fontWeight: FontWeight.w600, height: 1.2, letterSpacing: -0.4),
                      ),
                      const SizedBox(height: 28),
                      for (var i = 0; i < steps.length; i++) _StepRow(index: i, label: steps[i], state: i < step ? 1 : (i == step ? 0 : -1)),
                      const Spacer(),
                    ],
                  ),
                ),
              ),
            ),
          ),
          Expanded(flex: 6, child: form),
        ],
      ),
    );
  }
}

class _StepRow extends StatelessWidget {
  const _StepRow({required this.index, required this.label, required this.state});

  final int index;
  final String label;

  /// 1 done, 0 current, -1 upcoming.
  final int state;

  @override
  Widget build(BuildContext context) {
    final done = state == 1, current = state == 0;
    return Padding(
      padding: const EdgeInsets.only(bottom: 16),
      child: Row(children: [
        Container(
          width: 34,
          height: 34,
          alignment: Alignment.center,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            color: done ? AppColors.ok : (current ? AppColors.primary : Colors.white.withOpacity(0.08)),
            border: Border.all(color: Colors.white.withOpacity(current || done ? 0 : 0.15)),
          ),
          child: done
              ? const Icon(Icons.check, size: 18, color: Colors.white)
              : Text('${index + 1}', style: TextStyle(color: current ? Colors.white : Colors.white60, fontWeight: FontWeight.w600)),
        ),
        const SizedBox(width: 14),
        Text(label, style: TextStyle(color: current ? Colors.white : Colors.white70, fontSize: 16, fontWeight: current ? FontWeight.w600 : FontWeight.w400)),
      ]),
    );
  }
}

class _Heading extends StatelessWidget {
  const _Heading({required this.title, required this.subtitle});

  final String title;
  final String subtitle;

  @override
  Widget build(BuildContext context) => Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          if (MediaQuery.sizeOf(context).width < 900) ...[const BrandMark(), const SizedBox(height: 24)],
          Text(title, style: Theme.of(context).textTheme.headlineMedium),
          const SizedBox(height: 8),
          Text(subtitle, style: const TextStyle(fontSize: 15, color: AppColors.textMuted, height: 1.45)),
          const SizedBox(height: 28),
        ],
      );
}

/// First run: where is the server and which organisation (realm)?
class ServerSetupScreen extends StatefulWidget {
  const ServerSetupScreen({super.key});

  @override
  State<ServerSetupScreen> createState() => _ServerSetupScreenState();
}

class _ServerSetupScreenState extends State<ServerSetupScreen> {
  final _form = GlobalKey<FormState>();
  late final TextEditingController _server;
  late final TextEditingController _realm;
  late final TextEditingController _keycloak;
  bool _advanced = false;

  @override
  void initState() {
    super.initState();
    final cfg = context.read<DeviceSession>().config;
    _server = TextEditingController(text: cfg?.serverUrl ?? ServerConfig.presetServer);
    _realm = TextEditingController(text: cfg?.realm ?? ServerConfig.presetRealm);
    _keycloak = TextEditingController(text: cfg?.keycloakUrl ?? ServerConfig.presetKeycloak);
    _advanced = _keycloak.text.isNotEmpty;
  }

  @override
  void dispose() {
    _server.dispose();
    _realm.dispose();
    _keycloak.dispose();
    super.dispose();
  }

  /// Cleartext is refused (MOB-8): the device password and PINs travel there.
  static String? _https(S s, String? v) => TrustStore.isAllowed(ServerConfig.normalizeUrl(v ?? '')) ? null : s.t('setup.httpsRequired');

  void _submit() {
    if (!_form.currentState!.validate()) return;
    final kc = _keycloak.text.trim();
    context.read<DeviceSession>().saveServer(ServerConfig(
          serverUrl: ServerConfig.normalizeUrl(_server.text),
          realm: _realm.text.trim(),
          keycloakUrl: kc.isEmpty ? null : ServerConfig.normalizeUrl(kc),
        ));
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    String? req(String? v) => (v == null || v.trim().isEmpty) ? s.t('common.required') : null;
    return OnboardingFrame(
      step: 0,
      child: Form(
        key: _form,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            _Heading(title: s.t('setup.title'), subtitle: s.t('setup.subtitle')),
            TextFormField(
              controller: _server,
              keyboardType: TextInputType.url,
              autocorrect: false,
              decoration: InputDecoration(labelText: s.t('setup.server'), hintText: s.t('setup.serverHint'), prefixIcon: const Icon(Icons.dns_outlined)),
              validator: (v) => req(v) ?? _https(s, v),
            ),
            const SizedBox(height: 16),
            TextFormField(
              controller: _realm,
              autocorrect: false,
              decoration: InputDecoration(labelText: s.t('setup.realm'), hintText: s.t('setup.realmHint'), prefixIcon: const Icon(Icons.apartment_outlined)),
              validator: req,
              onFieldSubmitted: (_) => _submit(),
            ),
            const SizedBox(height: 8),
            Align(
              alignment: Alignment.centerLeft,
              child: TextButton.icon(
                onPressed: () => setState(() => _advanced = !_advanced),
                icon: Icon(_advanced ? Icons.expand_less : Icons.expand_more),
                label: Text(s.t('setup.advanced')),
              ),
            ),
            if (_advanced)
              TextFormField(
                controller: _keycloak,
                keyboardType: TextInputType.url,
                autocorrect: false,
                decoration: InputDecoration(labelText: s.t('setup.keycloak'), helperText: s.t('setup.keycloakHint')),
                validator: (v) => (v == null || v.trim().isEmpty) ? null : _https(s, v),
              ),
            const SizedBox(height: 24),
            FilledButton(onPressed: _submit, child: Text(s.t('setup.continue'))),
            const SizedBox(height: 20),
            const _LanguageSwitch(),
          ],
        ),
      ),
    );
  }
}

/// Device account sign-in + registration.
class DeviceLoginScreen extends StatefulWidget {
  const DeviceLoginScreen({super.key});

  @override
  State<DeviceLoginScreen> createState() => _DeviceLoginScreenState();
}

class _DeviceLoginScreenState extends State<DeviceLoginScreen> {
  final _form = GlobalKey<FormState>();
  final _user = TextEditingController();
  final _pass = TextEditingController();
  final _name = TextEditingController();
  bool _busy = false;
  bool _hide = true;
  String? _error;

  @override
  void dispose() {
    _user.dispose();
    _pass.dispose();
    _name.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    if (!_form.currentState!.validate()) return;
    final s = S.of(context);
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await context.read<DeviceSession>().login(_user.text, _pass.text, _name.text);
    } on LoginFailure catch (e) {
      _error = switch (e.reason) {
        'bad_credentials' => s.t('login.badCredentials'),
        'not_device' => s.t('login.notDevice'),
        'unreachable' => s.t('login.unreachable'),
        'insecure' => s.t('login.insecure'),
        _ => '${s.t('common.error')} (${e.detail ?? e.reason})',
      };
    } on ApiException catch (e) {
      _error = e.network ? s.t('login.unreachable') : s.error(e.code, '${s.t('common.error')}: ${e.message}');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final session = context.watch<DeviceSession>();
    String? req(String? v) => (v == null || v.trim().isEmpty) ? s.t('common.required') : null;
    return OnboardingFrame(
      step: 1,
      child: AutofillGroup(
        child: Form(
          key: _form,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              _Heading(title: s.t('login.title'), subtitle: s.t('login.subtitle')),
              _ServerChip(config: session.config, onEdit: () => _changeServer(context)),
              const SizedBox(height: 20),
              TextFormField(
                controller: _name,
                textCapitalization: TextCapitalization.words,
                decoration: InputDecoration(labelText: s.t('login.deviceName'), hintText: s.t('login.deviceNameHint'), prefixIcon: const Icon(Icons.tablet_android_outlined)),
                validator: req,
              ),
              const SizedBox(height: 16),
              TextFormField(
                controller: _user,
                autocorrect: false,
                autofillHints: const [AutofillHints.username],
                decoration: InputDecoration(labelText: s.t('login.username'), prefixIcon: const Icon(Icons.person_outline)),
                validator: req,
              ),
              const SizedBox(height: 16),
              TextFormField(
                controller: _pass,
                obscureText: _hide,
                autofillHints: const [AutofillHints.password],
                decoration: InputDecoration(
                  labelText: s.t('login.password'),
                  prefixIcon: const Icon(Icons.lock_outline),
                  suffixIcon: IconButton(icon: Icon(_hide ? Icons.visibility_outlined : Icons.visibility_off_outlined), onPressed: () => setState(() => _hide = !_hide)),
                ),
                validator: req,
                onFieldSubmitted: (_) => _submit(),
              ),
              if (_error != null) ...[
                const SizedBox(height: 16),
                Container(
                  padding: const EdgeInsets.all(14),
                  decoration: BoxDecoration(color: AppColors.dangerBg, borderRadius: BorderRadius.circular(12), border: Border.all(color: const Color(0xFFFECACA))),
                  child: Row(children: [
                    const Icon(Icons.error_outline, color: AppColors.danger),
                    const SizedBox(width: 10),
                    Expanded(child: Text(_error!, style: const TextStyle(color: Color(0xFF991B1B)))),
                  ]),
                ),
              ],
              const SizedBox(height: 24),
              FilledButton(
                onPressed: _busy ? null : _submit,
                child: _busy ? const SizedBox(width: 22, height: 22, child: CircularProgressIndicator(strokeWidth: 2.5, color: Colors.white)) : Text(s.t('login.submit')),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// Changing the server of a tablet that was in use needs a supervisor, and
/// is refused while items are queued for the current server (MOB-2).
Future<void> _changeServer(BuildContext context) async {
  final s = S.of(context);
  final session = context.read<DeviceSession>();
  final n = await session.db.outboxCount();
  if (!context.mounted) return;
  if (n > 0) return showOutboxBlocked(context, s.t('dev.serverBlocked'));
  if (session.roster.isNotEmpty && !await requireSupervisor(context)) return;
  try {
    await session.editServer();
  } on OutboxNotEmpty {
    if (context.mounted) await showOutboxBlocked(context, s.t('dev.serverBlocked'));
  }
}

class _ServerChip extends StatelessWidget {
  const _ServerChip({required this.config, required this.onEdit});

  final ServerConfig? config;
  final VoidCallback onEdit;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    return Container(
      padding: const EdgeInsets.fromLTRB(14, 10, 6, 10),
      decoration: BoxDecoration(color: AppColors.bg, borderRadius: BorderRadius.circular(12), border: Border.all(color: AppColors.border)),
      child: Row(children: [
        const Icon(Icons.dns_outlined, size: 20, color: AppColors.textMuted),
        const SizedBox(width: 10),
        Expanded(
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Text(config?.serverUrl ?? '', style: const TextStyle(fontWeight: FontWeight.w600), overflow: TextOverflow.ellipsis),
            Text(config?.realm ?? '', style: const TextStyle(fontSize: 13, color: AppColors.textMuted)),
          ]),
        ),
        TextButton(onPressed: onEdit, child: Text(s.t('login.changeServer'))),
      ]),
    );
  }
}

class _LanguageSwitch extends StatelessWidget {
  const _LanguageSwitch();

  @override
  Widget build(BuildContext context) {
    final session = context.watch<DeviceSession>();
    return Center(
      child: SegmentedButton<String>(
        segments: const [ButtonSegment(value: 'id', label: Text('Indonesia')), ButtonSegment(value: 'en', label: Text('English'))],
        selected: {session.locale.languageCode},
        showSelectedIcon: false,
        onSelectionChanged: (v) => session.setLanguage(v.first),
      ),
    );
  }
}
