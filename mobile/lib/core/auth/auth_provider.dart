import 'dart:async';
import 'dart:convert';

import 'package:flutter/widgets.dart';

import '../../config/env.dart';
import '../../shared/models/models.dart';
import '../api/api_client.dart';
import '../database/local_db.dart';
import '../storage/secure_store.dart';
import 'keycloak_service.dart';

/// Where the tablet is in its life cycle.
enum Phase { booting, setup, login, pending, revoked, locked, unlocked }

/// The tablet's session: server config, device registration (D-001), site
/// reference data cached for offline use, and which guard is using it.
class DeviceSession extends ChangeNotifier with WidgetsBindingObserver {
  DeviceSession({required this.store, required this.auth, required this.db});

  final SecureStore store;
  final KeycloakService auth;
  final LocalDb db;

  late final ApiClient api = ApiClient(auth, () => config!);

  Phase phase = Phase.booting;
  ServerConfig? config;
  Device? device;
  Site? site;
  SiteConfig siteConfig = const SiteConfig();
  List<RosterGuard> roster = const [];
  List<PatrolRoute> routes = const [];
  List<SosAlert> sosAlerts = const [];
  RosterGuard? guard;
  GuardStatus? guardStatus;
  DateTime? referenceSyncedAt;
  Locale locale = const Locale('id');

  /// Set by the patrol feature so auto-lock is relaxed during a running patrol.
  bool patrolRunning = false;

  DateTime _lastActivity = DateTime.now();
  Timer? _pendingPoll;
  Timer? _ticker;
  int _tick = 0;

  // ── boot & setup ───────────────────────────────────────────────────────────

  Future<void> boot() async {
    WidgetsBinding.instance.addObserver(this);
    final lang = await store.read(SecureKeys.language);
    if (lang == 'en') locale = const Locale('en');
    final raw = await store.read(SecureKeys.config);
    config = raw == null ? null : ServerConfig.fromJson((jsonDecode(raw) as Map).cast<String, dynamic>());
    if (config == null) return _go(Phase.setup);
    if (!await auth.hasSession) return _go(Phase.login);

    await _loadCache();
    _go(device?.isActive == true ? Phase.locked : (device?.status == DeviceStatusKind.revoked ? Phase.revoked : Phase.pending));
    _startTicker();
    unawaited(refreshDevice());
  }

  Future<void> saveServer(ServerConfig cfg) async {
    config = cfg;
    await store.write(SecureKeys.config, jsonEncode(cfg.toJson()));
    _go(Phase.login);
  }

  /// Back from login to the server setup screen.
  void editServer() => _go(Phase.setup);

  Future<void> setLanguage(String code) async {
    locale = Locale(code);
    await store.write(SecureKeys.language, code);
    notifyListeners();
  }

  /// Signs in with the device account and registers the tablet.
  Future<void> login(String username, String password, String deviceName) async {
    await auth.login(config!, username, password);
    final res = await api.post('/device/register', body: {'name': deviceName.trim()});
    device = Device.fromJson((res as Map).cast<String, dynamic>());
    await db.put(CacheKeys.device, device!.toJson());
    _startTicker();
    await refreshDevice();
    if (phase == Phase.login) _go(Phase.pending);
  }

  // ── device state ───────────────────────────────────────────────────────────

  /// Re-reads the device record; moves between pending/active/revoked.
  Future<void> refreshDevice() async {
    try {
      final res = (await api.get('/device/me') as Map).cast<String, dynamic>();
      device = Device.fromJson((res['device'] as Map).cast<String, dynamic>());
      await db.put(CacheKeys.device, device!.toJson());
      if (res['site'] is Map) {
        site = Site.fromJson((res['site'] as Map).cast<String, dynamic>());
        await db.put(CacheKeys.site, site!.toJson());
      }
      if (res['config'] is Map) {
        siteConfig = SiteConfig.fromJson((res['config'] as Map).cast<String, dynamic>());
        await db.put(CacheKeys.siteConfig, siteConfig.toJson());
      }
      if (device!.status == DeviceStatusKind.revoked) {
        _go(Phase.revoked);
      } else if (!device!.isActive) {
        _go(Phase.pending);
      } else {
        if (phase == Phase.pending || phase == Phase.revoked || phase == Phase.login || phase == Phase.booting) {
          await syncReference();
          _go(Phase.locked);
        }
      }
    } on ApiException catch (e) {
      if (e.code == 'auth_expired') return expire();
      if (e.code == 'device_unregistered' && device != null) {
        // Registration was deleted on the server: register again.
        try {
          await api.post('/device/register', body: {'name': device!.name});
        } catch (_) {}
      }
      // Offline: keep using the cache.
    }
  }

  /// Roster (with PIN hashes) and routes, cached for offline use.
  Future<void> syncReference() async {
    try {
      final r = (await api.get('/device/roster') as Map).cast<String, dynamic>();
      roster = ((r['guards'] as List?) ?? const []).map((e) => RosterGuard.fromJson((e as Map).cast<String, dynamic>())).toList()
        ..sort((a, b) => a.name.toLowerCase().compareTo(b.name.toLowerCase()));
      await db.put(CacheKeys.roster, roster.map((g) => g.toJson()).toList());
      final rt = await api.get('/device/routes');
      routes = ((rt as List?) ?? const []).map((e) => PatrolRoute.fromJson((e as Map).cast<String, dynamic>())).toList();
      await db.put(CacheKeys.routes, routes.map((r) => r.toJson()).toList());
      referenceSyncedAt = DateTime.now();
      // A guard removed or deactivated on the server loses access right away.
      if (guard != null && !roster.any((g) => g.id == guard!.id)) lock();
      notifyListeners();
    } on ApiException catch (e) {
      if (e.code == 'auth_expired') return expire();
      if (e.status == 403 && (e.code ?? '').startsWith('device_')) await refreshDevice();
    }
  }

  Future<void> _loadCache() async {
    final d = await db.getMap(CacheKeys.device);
    if (d != null) device = Device.fromJson(d);
    final s = await db.getMap(CacheKeys.site);
    if (s != null) site = Site.fromJson(s);
    final c = await db.getMap(CacheKeys.siteConfig);
    if (c != null) siteConfig = SiteConfig.fromJson(c);
    roster = (await db.getList(CacheKeys.roster)).map(RosterGuard.fromJson).toList();
    routes = (await db.getList(CacheKeys.routes)).map(PatrolRoute.fromJson).toList();
  }

  // ── guard lock / unlock ────────────────────────────────────────────────────

  /// Called by the lock screen after the PIN was verified locally.
  Future<void> unlock(RosterGuard g) async {
    guard = g;
    guardStatus = null;
    _lastActivity = DateTime.now();
    final cached = await db.getMap(CacheKeys.guardStatus(g.id));
    if (cached != null) guardStatus = GuardStatus.fromJson(cached);
    _go(Phase.unlocked);
    unawaited(refreshGuardStatus());
  }

  void lock() {
    guard = null;
    guardStatus = null;
    patrolRunning = false;
    if (phase == Phase.unlocked) _go(Phase.locked);
  }

  /// Records user activity (any touch) for the idle auto-lock.
  void touch() => _lastActivity = DateTime.now();

  Duration get idleLimit {
    var limit = Duration(minutes: siteConfig.idleLockMinutes.clamp(1, 120));
    if (patrolRunning && limit < const Duration(minutes: 30)) limit = const Duration(minutes: 30);
    // Typing on the soft keyboard produces no pointer events: give a guard who
    // is writing a report time to finish instead of locking and losing it.
    if (_editingText && limit < const Duration(minutes: 15)) limit = const Duration(minutes: 15);
    return limit;
  }

  static bool get _editingText {
    final ctx = FocusManager.instance.primaryFocus?.context;
    return ctx != null && ctx.findAncestorWidgetOfExactType<EditableText>() != null;
  }

  Future<void> refreshGuardStatus() async {
    final g = guard;
    if (g == null) return;
    try {
      final res = (await api.get('/device/status', guardId: g.id) as Map).cast<String, dynamic>();
      if (guard?.id != g.id) return;
      guardStatus = GuardStatus.fromJson(res);
      await db.put(CacheKeys.guardStatus(g.id), res);
      notifyListeners();
    } on ApiException catch (e) {
      if (e.code == 'auth_expired') return expire();
      if (e.code == 'guard_not_allowed' || e.code == 'guard_unknown') {
        await syncReference();
        lock();
      }
    }
  }

  Future<void> refreshSos() async {
    try {
      final res = await api.get('/device/sos');
      sosAlerts = ((res as List?) ?? const []).map((e) => SosAlert.fromJson((e as Map).cast<String, dynamic>())).toList();
      notifyListeners();
    } on ApiException catch (e) {
      if (e.code == 'auth_expired') return expire();
    }
  }

  // ── timers ─────────────────────────────────────────────────────────────────

  void _startTicker() {
    _ticker?.cancel();
    _ticker = Timer.periodic(const Duration(seconds: 5), (_) => _onTick());
  }

  void _onTick() {
    _tick++;
    if (phase == Phase.unlocked && DateTime.now().difference(_lastActivity) > idleLimit) lock();
    if (phase == Phase.pending && _tick % 2 == 0) unawaited(refreshDevice()); // every 10 s
    if ((phase == Phase.locked || phase == Phase.unlocked) && _tick % 3 == 0) unawaited(refreshSos()); // every 15 s
    if ((phase == Phase.locked || phase == Phase.unlocked) && _tick % 60 == 0) unawaited(syncReference()); // every 5 min
    if (phase == Phase.unlocked && _tick % 12 == 0) unawaited(refreshGuardStatus()); // every minute
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed && phase == Phase.unlocked && DateTime.now().difference(_lastActivity) > idleLimit) lock();
  }

  // ── sign-out / expiry ──────────────────────────────────────────────────────

  /// The offline token is no longer valid (device account disabled or the
  /// session revoked): sign in again.
  Future<void> expire() async {
    // Already signed out (e.g. the outbox reports it again on every tick):
    // don't pull the operator away from the setup/login screens.
    if (phase == Phase.setup || phase == Phase.login || phase == Phase.booting) return;
    await auth.signOut();
    guard = null;
    _go(Phase.login);
  }

  /// Removes the device account and all local data from the tablet.
  Future<void> signOutDevice() async {
    await auth.signOut();
    await db.wipe();
    device = null;
    site = null;
    roster = const [];
    routes = const [];
    guard = null;
    _go(Phase.login);
  }

  void _go(Phase p) {
    phase = p;
    _pendingPoll?.cancel();
    notifyListeners();
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _ticker?.cancel();
    _pendingPoll?.cancel();
    super.dispose();
  }
}
