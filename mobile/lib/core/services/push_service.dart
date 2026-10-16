import 'dart:async';
import 'dart:io';
import 'dart:ui' show DartPluginRegistrant;

import 'package:firebase_core/firebase_core.dart';
import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_local_notifications/flutter_local_notifications.dart';
import 'package:path_provider/path_provider.dart';

import '../../l10n/strings.dart';
import '../api/api_client.dart';
import '../auth/auth_provider.dart';

// Push notifications through Firebase Cloud Messaging: the server sends
// high-priority *data* messages so a tablet reacts at once (an SOS from
// another post, approval/revocation, roster changes) instead of waiting for
// its next poll. Push only speeds things up — polling continues (slower while
// push is healthy, never slower than every 60 s for SOS).
//
// Firebase is initialised from `--dart-define` values (no
// google-services.json, no Google Services Gradle plugin). Without them the
// app runs exactly as before: push disabled, normal polling.

/// Firebase project values from the build (`--dart-define=SP_FCM_*`).
class PushConfig {
  const PushConfig({this.apiKey = '', this.appId = '', this.projectId = '', this.senderId = '', this.iosAppId = ''});

  final String apiKey;

  /// Android app id (`1:<sender>:android:<hash>`).
  final String appId;
  final String projectId;
  final String senderId;

  /// iOS app id (optional; push stays off on iOS without it).
  final String iosAppId;

  static const build = PushConfig(
    apiKey: String.fromEnvironment('SP_FCM_API_KEY'),
    appId: String.fromEnvironment('SP_FCM_APP_ID'),
    projectId: String.fromEnvironment('SP_FCM_PROJECT_ID'),
    senderId: String.fromEnvironment('SP_FCM_SENDER_ID'),
    iosAppId: String.fromEnvironment('SP_FCM_IOS_APP_ID'),
  );

  String get platformAppId => !kIsWeb && Platform.isIOS ? iosAppId : appId;

  bool get configured => apiKey.isNotEmpty && platformAppId.isNotEmpty && projectId.isNotEmpty && senderId.isNotEmpty;

  FirebaseOptions get options => FirebaseOptions(apiKey: apiKey, appId: platformAppId, messagingSenderId: senderId, projectId: projectId);
}

/// Message types of the server (`data.type`).
enum PushKind { sos, sosAck, patrolNotStarted, deviceStatus, sync, unknown }

/// One FCM data message: `{type, site_id, ref_id, title, body}` (all strings).
class PushMessage {
  const PushMessage({required this.kind, this.type = '', this.siteId = '', this.refId = '', this.title = '', this.body = ''});

  factory PushMessage.fromData(Map<String, dynamic> data) {
    String s(String k) => data[k] == null ? '' : '${data[k]}'.trim();
    final type = s('type');
    return PushMessage(
      kind: switch (type) {
        'sos' => PushKind.sos,
        'sos_ack' => PushKind.sosAck,
        'patrol_not_started' => PushKind.patrolNotStarted,
        'device_status' => PushKind.deviceStatus,
        'sync' => PushKind.sync,
        _ => PushKind.unknown,
      },
      type: type,
      siteId: s('site_id'),
      refId: s('ref_id'),
      title: s('title'),
      body: s('body'),
    );
  }

  final PushKind kind;
  final String type;
  final String siteId;
  final String refId;
  final String title;
  final String body;

  /// What the app must reload after this message; stored by the background
  /// handler so the app catches up when it comes back to the foreground.
  String? get refreshFlag => switch (kind) {
        PushKind.sos || PushKind.sosAck => PushFlags.sos,
        PushKind.patrolNotStarted => PushFlags.status,
        PushKind.deviceStatus => PushFlags.device,
        PushKind.sync => PushFlags.sync,
        PushKind.unknown => null,
      };

  /// Where tapping a notification about this message leads.
  String? get tapPayload => switch (kind) {
        PushKind.sos || PushKind.sosAck => 'sos:$refId',
        PushKind.patrolNotStarted => 'home',
        _ => null,
      };
}

/// Refresh flags left by the background handler.
class PushFlags {
  static const sos = 'sos';
  static const status = 'status';
  static const device = 'device';
  static const sync = 'sync';
}

/// Android notification channels.
class PushChannels {
  static const sos = 'garda_sos';
  static const patrol = 'garda_patrol';
  static const system = 'garda_system';
}

/// A local notification to show.
class PushNote {
  const PushNote({required this.id, required this.channel, required this.title, this.body, required this.payload});

  final int id;
  final String channel;
  final String title;
  final String? body;
  final String payload;
}

/// Stable notification id for [key] (FNV-1a, 31 bit) — the same in the
/// background isolate and the app, so an acknowledgement cancels the SOS.
int notificationId(String key) {
  var h = 0x811c9dc5;
  for (final c in key.codeUnits) {
    h ^= c;
    h = (h * 0x01000193) & 0xffffffff;
  }
  return h & 0x7fffffff;
}

int sosNotificationId(String refId) => notificationId('sos:$refId');

/// The notification for [m], or null for types that only refresh data.
PushNote? noteFor(PushMessage m, {String patrolTitle = 'GARDA'}) {
  switch (m.kind) {
    case PushKind.sos:
      final what = m.body.isNotEmpty ? m.body : m.title;
      return PushNote(
        id: sosNotificationId(m.refId),
        channel: PushChannels.sos,
        title: what.isEmpty ? 'SOS' : 'SOS · $what',
        body: m.body.isNotEmpty && m.title.isNotEmpty ? m.title : null,
        payload: m.tapPayload!,
      );
    case PushKind.patrolNotStarted:
      return PushNote(
        id: notificationId('patrol:${m.refId}'),
        channel: PushChannels.patrol,
        title: m.title.isNotEmpty ? m.title : patrolTitle,
        body: m.body.isEmpty ? null : m.body,
        payload: m.tapPayload!,
      );
    default:
      return null;
  }
}

// ── platform seams (faked in tests) ─────────────────────────────────────────

enum PushPermission { granted, denied, notDetermined }

/// The messaging service (Firebase Cloud Messaging in the app).
abstract class PushTransport {
  Future<void> init(PushConfig config);
  Future<PushPermission> permission();

  /// Android 13+ POST_NOTIFICATIONS / iOS alert+sound+badge.
  Future<PushPermission> requestPermission();
  Future<String?> token();
  Stream<String> get tokenRefresh;

  /// Data of messages received while the app is in the foreground.
  Stream<Map<String, dynamic>> get messages;

  /// Data of system-shown (notification-type) messages the user tapped.
  Stream<Map<String, dynamic>> get opened;
  Future<Map<String, dynamic>?> initialMessage();
  Future<void> deleteToken();
}

class FirebasePushTransport implements PushTransport {
  FirebaseMessaging get _m => FirebaseMessaging.instance;

  static PushPermission _map(AuthorizationStatus s) => switch (s) {
        AuthorizationStatus.authorized || AuthorizationStatus.provisional => PushPermission.granted,
        AuthorizationStatus.denied || AuthorizationStatus.deniedPermanently => PushPermission.denied,
        AuthorizationStatus.notDetermined => PushPermission.notDetermined,
      };

  @override
  Future<void> init(PushConfig config) async {
    await Firebase.initializeApp(options: config.options);
    FirebaseMessaging.onBackgroundMessage(pushBackgroundHandler);
  }

  @override
  Future<PushPermission> permission() async => _map((await _m.getNotificationSettings()).authorizationStatus);

  @override
  Future<PushPermission> requestPermission() async => _map((await _m.requestPermission(alert: true, badge: true, sound: true)).authorizationStatus);

  // firebase_messaging 17 deprecates registration tokens in favour of
  // register()/onRegistered (Firebase Installation IDs for "direct send").
  // The backend sends through the FCM HTTP v1 API with registration tokens
  // (D-057), so keep them until the backend contract is migrated.
  @override
  // ignore: deprecated_member_use
  Future<String?> token() => _m.getToken();

  @override
  // ignore: deprecated_member_use
  Stream<String> get tokenRefresh => _m.onTokenRefresh;

  @override
  Stream<Map<String, dynamic>> get messages => FirebaseMessaging.onMessage.map((m) => m.data);

  @override
  Stream<Map<String, dynamic>> get opened => FirebaseMessaging.onMessageOpenedApp.map((m) => m.data);

  @override
  Future<Map<String, dynamic>?> initialMessage() async => (await _m.getInitialMessage())?.data;

  @override
  // ignore: deprecated_member_use
  Future<void> deleteToken() => _m.deleteToken();
}

/// Shows local notifications.
abstract class PushNotifier {
  Future<void> init({void Function(String payload)? onTap, Map<String, String> channelDescriptions = const {}});
  Future<void> show(PushNote note);
  Future<void> cancel(int id);

  /// Payload of the notification that launched the app, if any.
  Future<String?> launchPayload();
}

class LocalPushNotifier implements PushNotifier {
  LocalPushNotifier([FlutterLocalNotificationsPlugin? plugin]) : _p = plugin ?? FlutterLocalNotificationsPlugin();

  final FlutterLocalNotificationsPlugin _p;
  Map<String, String> _descriptions = const {};

  /// Monochrome status-bar icon (`res/drawable/ic_stat_garda.xml`, kept in
  /// release builds by `res/raw/keep.xml`).
  static const icon = 'ic_stat_garda';

  static bool get _supported => !kIsWeb && (Platform.isAndroid || Platform.isIOS);

  static final _sosVibration = Int64List.fromList([0, 900, 400, 900, 400, 900]);

  static AndroidNotificationChannel _channel(String id, Map<String, String> d) => switch (id) {
        PushChannels.sos => AndroidNotificationChannel(PushChannels.sos, 'SOS',
            description: d[id],
            importance: Importance.max,
            playSound: true,
            enableVibration: true,
            vibrationPattern: _sosVibration,
            audioAttributesUsage: AudioAttributesUsage.alarm),
        PushChannels.patrol => AndroidNotificationChannel(PushChannels.patrol, 'Patroli', description: d[id], importance: Importance.defaultImportance),
        _ => AndroidNotificationChannel(PushChannels.system, 'Sistem', description: d[id], importance: Importance.low, playSound: false, enableVibration: false),
      };

  @override
  Future<void> init({void Function(String payload)? onTap, Map<String, String> channelDescriptions = const {}}) async {
    _descriptions = channelDescriptions;
    if (!_supported) return;
    await _p.initialize(
      settings: const InitializationSettings(
        android: AndroidInitializationSettings(icon),
        iOS: DarwinInitializationSettings(requestAlertPermission: false, requestBadgePermission: false, requestSoundPermission: false),
      ),
      onDidReceiveNotificationResponse: onTap == null
          ? null
          : (r) {
              final p = r.payload;
              if (p != null && p.isNotEmpty) onTap(p);
            },
    );
    final android = _p.resolvePlatformSpecificImplementation<AndroidFlutterLocalNotificationsPlugin>();
    for (final id in const [PushChannels.sos, PushChannels.patrol, PushChannels.system]) {
      await android?.createNotificationChannel(_channel(id, channelDescriptions));
    }
  }

  @override
  Future<void> show(PushNote n) async {
    if (!_supported) return;
    final ch = _channel(n.channel, _descriptions);
    final sos = n.channel == PushChannels.sos;
    await _p.show(
      id: n.id,
      title: n.title,
      body: n.body,
      notificationDetails: NotificationDetails(
        android: AndroidNotificationDetails(
          ch.id,
          ch.name,
          channelDescription: ch.description,
          icon: icon,
          importance: ch.importance,
          priority: sos ? Priority.max : Priority.defaultPriority,
          playSound: ch.playSound,
          enableVibration: ch.enableVibration,
          vibrationPattern: ch.vibrationPattern,
          audioAttributesUsage: ch.audioAttributesUsage,
          category: sos ? AndroidNotificationCategory.alarm : AndroidNotificationCategory.reminder,
          visibility: sos ? NotificationVisibility.public : NotificationVisibility.private,
          ticker: n.title,
        ),
        iOS: const DarwinNotificationDetails(presentAlert: true, presentSound: true, presentBanner: true, presentList: true),
      ),
      payload: n.payload,
    );
  }

  @override
  Future<void> cancel(int id) async {
    if (_supported) await _p.cancel(id: id);
  }

  @override
  Future<String?> launchPayload() async {
    if (!_supported) return null;
    final d = await _p.getNotificationAppLaunchDetails();
    return d?.didNotificationLaunchApp == true ? d!.notificationResponse?.payload : null;
  }
}

/// Refresh flags written by the background isolate. A tiny plain file, not
/// the encrypted kv cache: opening that database from a second isolate would
/// mean a second SQLite connection and loading the field key (which
/// [FieldCipher.load] creates when it cannot be read — the app would then
/// drop its data). The flags hold no personal data.
abstract class PushFlagStore {
  Future<void> add(String flag);

  /// Returns and clears the pending flags.
  Future<Set<String>> take();
}

class FilePushFlagStore implements PushFlagStore {
  Future<File> _file() async => File('${(await getApplicationSupportDirectory()).path}${Platform.pathSeparator}push_pending');

  static Future<Set<String>> _read(File f) async {
    if (!await f.exists()) return <String>{};
    return (await f.readAsString()).split(',').map((e) => e.trim()).where((e) => e.isNotEmpty).toSet();
  }

  @override
  Future<void> add(String flag) async {
    final f = await _file();
    final cur = await _read(f)..add(flag);
    await f.writeAsString(cur.join(','), flush: true);
  }

  @override
  Future<Set<String>> take() async {
    final f = await _file();
    final cur = await _read(f);
    if (cur.isNotEmpty) await f.delete();
    return cur;
  }
}

class MemoryPushFlagStore implements PushFlagStore {
  final Set<String> flags = {};

  @override
  Future<void> add(String flag) async => flags.add(flag);

  @override
  Future<Set<String>> take() async {
    final out = {...flags};
    flags.clear();
    return out;
  }
}

/// FCM background handler (app in the background or not running). Runs in
/// its own isolate without app state: show the notification and leave a
/// refresh flag for the app.
@pragma('vm:entry-point')
Future<void> pushBackgroundHandler(RemoteMessage message) async {
  try {
    DartPluginRegistrant.ensureInitialized();
    final m = PushMessage.fromData(message.data);
    final flag = m.refreshFlag;
    if (flag == null) return; // unknown type
    try {
      await FilePushFlagStore().add(flag);
    } catch (e) {
      debugPrint('push: flag not stored: $e');
    }
    final note = noteFor(m);
    if (note == null && m.kind != PushKind.sosAck) return;
    final notifier = LocalPushNotifier();
    await notifier.init();
    if (m.kind == PushKind.sosAck) {
      await notifier.cancel(sosNotificationId(m.refId));
    } else {
      await notifier.show(note!);
    }
  } catch (e) {
    debugPrint('push background: $e');
  }
}

// ── the service ─────────────────────────────────────────────────────────────

/// Push status shown on the Device Info screen and sent in the health beacon.
enum PushState { unconfigured, denied, tokenPending, active, error }

class PushService extends ChangeNotifier with WidgetsBindingObserver {
  PushService({
    required this.session,
    this.config = PushConfig.build,
    PushTransport? transport,
    PushNotifier? notifier,
    PushFlagStore? flags,
    this.retryDelays = const [Duration(seconds: 15), Duration(seconds: 30), Duration(minutes: 1), Duration(minutes: 2), Duration(minutes: 5), Duration(minutes: 10)],
    String? platform,
  })  : _transport = transport ?? (config.configured ? FirebasePushTransport() : null),
        _notifier = notifier ?? LocalPushNotifier(),
        _flags = flags ?? FilePushFlagStore(),
        platform = platform ?? (!kIsWeb && Platform.isIOS ? 'ios' : 'android');

  final DeviceSession session;
  final PushConfig config;
  final PushTransport? _transport;
  final PushNotifier _notifier;
  final PushFlagStore _flags;
  final List<Duration> retryDelays;

  /// `platform` of `PUT /device/push-token`.
  final String platform;

  /// Opens what a tapped notification points at (wired in main).
  void Function(String payload)? onOpen;

  /// Errors worth keeping in the local error log (wired in main).
  void Function(Object error, StackTrace? stack)? onError;

  PushPermission permission = PushPermission.notDetermined;
  Object? lastError;

  bool _started = false;
  bool _initFailed = false;
  String? _token;
  String? _registeredToken;
  Timer? _retry;
  int _attempt = 0;
  bool _disposed = false;
  final _subs = <StreamSubscription<Object?>>[];

  static const _phasesWithDevice = {Phase.login, Phase.pending, Phase.locked, Phase.unlocked};

  bool get _ready => _started && !_initFailed && config.configured && _transport != null;

  /// Firebase is up: asking for permission / resending the token can help.
  bool get canRetry => _ready;

  /// The token the server has (null until `PUT /device/push-token` succeeded).
  String? get registeredToken => _registeredToken;

  PushState get state {
    if (!config.configured || _transport == null) return PushState.unconfigured;
    if (_initFailed) return PushState.error;
    if (!_started) return PushState.tokenPending;
    if (permission != PushPermission.granted) return PushState.denied;
    if (_token == null || _registeredToken != _token) return PushState.tokenPending;
    return PushState.active;
  }

  /// Configured, permission granted and the token registered: SOS polling
  /// may slow down (DeviceSession.sosPollInterval).
  bool get healthy => state == PushState.active;

  /// `push` of the health beacon.
  String get beaconStatus => switch (state) {
        PushState.active => 'active',
        PushState.unconfigured => 'unconfigured',
        PushState.denied => 'denied',
        PushState.tokenPending || PushState.error => 'error',
      };

  /// Connects the device session's hooks to this service.
  void attach() {
    session.pushHealthy = () => healthy;
    session.onRegistered = () => unawaited(onDeviceRegistered());
    session.onApproved = () => unawaited(onDeviceApproved());
    session.beforeSignOut = unregister;
  }

  /// Initialises Firebase and the notification channels, then registers the
  /// token. Does nothing (state `unconfigured`) without the build values.
  Future<void> start() async {
    if (_started) return;
    _started = true;
    final t = _transport;
    if (!config.configured || t == null) {
      _notify();
      return;
    }
    try {
      await t.init(config).timeout(const Duration(seconds: 20));
      final s = S(session.locale.languageCode);
      await _notifier.init(onTap: _open, channelDescriptions: {
        PushChannels.sos: s.t('push.channelSos'),
        PushChannels.patrol: s.t('push.channelPatrol'),
        PushChannels.system: s.t('push.channelSystem'),
      });
    } catch (e, st) {
      _initFailed = true;
      lastError = e;
      onError?.call(e, st);
      _notify();
      return;
    }
    _subs
      ..add(t.messages.listen((d) => unawaited(handle(PushMessage.fromData(d)))))
      ..add(t.opened.listen((d) {
        final p = PushMessage.fromData(d).tapPayload;
        if (p != null) _open(p);
      }))
      ..add(t.tokenRefresh.listen((token) {
        _token = token;
        unawaited(register());
      }));
    WidgetsBinding.instance.addObserver(this);
    permission = await _permission(t.permission);
    await _consumeFlags();
    String? launch;
    try {
      launch = await _notifier.launchPayload();
      launch ??= PushMessage.fromData(await t.initialMessage() ?? const {}).tapPayload;
    } catch (_) {}
    if (launch != null) _open(launch);
    await register();
    // Restarted while waiting for approval: the operator is at the tablet.
    if (session.phase == Phase.pending) await _askIfUndetermined();
    _notify();
  }

  /// Sends the token to the server (`PUT /device/push-token`); retried with
  /// backoff while it fails. Also called on token refresh and after every
  /// device (re)registration and approval.
  Future<void> register() async {
    final t = _transport;
    if (!_ready || t == null) return;
    if (session.device == null || !_phasesWithDevice.contains(session.phase)) return;
    _retry?.cancel();
    try {
      final token = _token ??= await t.token();
      if (token == null || token.isEmpty) throw StateError('no FCM token');
      await session.api.put('/device/push-token', body: {'token': token, 'platform': platform});
      _registeredToken = token;
      _attempt = 0;
      lastError = null;
    } on ApiException catch (e) {
      lastError = e;
      // Signed out or revoked: a new registration starts over.
      if (e.code != 'auth_expired' && e.code != 'device_revoked') _scheduleRetry(slow: !e.retryable);
    } catch (e, st) {
      // No Google Play services, FCM unreachable, …
      lastError = e;
      onError?.call(e, st);
      _scheduleRetry();
    }
    _notify();
  }

  void _scheduleRetry({bool slow = false}) {
    if (_disposed || retryDelays.isEmpty) return;
    final delay = slow ? const Duration(minutes: 30) : retryDelays[_attempt < retryDelays.length ? _attempt : retryDelays.length - 1];
    _attempt++;
    _retry?.cancel();
    _retry = Timer(delay, () => unawaited(register()));
  }

  /// Registration done on the device login screen (the operator is there):
  /// send the token and ask for the notification permission.
  Future<void> onDeviceRegistered() async {
    await register();
    await _askIfUndetermined();
  }

  /// An admin approved the tablet (or re-activated it).
  Future<void> onDeviceApproved() async {
    await register();
    await _askIfUndetermined();
  }

  Future<void> _askIfUndetermined() async {
    if (_ready && permission == PushPermission.notDetermined) await requestPermission();
  }

  /// Asks for the notification permission (Device Info: "ask again").
  Future<PushPermission> requestPermission() async {
    final t = _transport;
    if (!_ready || t == null) return permission;
    permission = await _permission(t.requestPermission);
    _notify();
    if (permission == PushPermission.granted && _registeredToken == null) unawaited(register());
    return permission;
  }

  /// Opens the app's notification settings (Android refuses a third prompt).
  Future<bool> openSettings() async {
    try {
      return await const MethodChannel('securepatrol/app').invokeMethod<bool>('openNotificationSettings') ?? false;
    } catch (_) {
      return false;
    }
  }

  Future<PushPermission> _permission(Future<PushPermission> Function() f) async {
    try {
      return await f();
    } catch (e) {
      lastError = e;
      return permission;
    }
  }

  /// Device sign-out: the server forgets the token (errors ignored) and the
  /// next device account gets a new one.
  Future<void> unregister() async {
    _retry?.cancel();
    _attempt = 0;
    try {
      await session.api.delete('/device/push-token', timeout: const Duration(seconds: 5)).timeout(const Duration(seconds: 8));
    } catch (_) {}
    final t = _transport;
    if (_ready && t != null) {
      try {
        await t.deleteToken().timeout(const Duration(seconds: 5));
      } catch (_) {}
    }
    _token = null;
    _registeredToken = null;
    _notify();
  }

  /// Reacts to one message (foreground; the background handler only shows
  /// the notification and leaves a flag).
  Future<void> handle(PushMessage m) async {
    if (m.kind == PushKind.unknown) return;
    final phase = session.phase;
    if (phase == Phase.setup || phase == Phase.login || phase == Phase.booting) return;
    final inUse = phase == Phase.locked || phase == Phase.unlocked;
    // A message about another site (e.g. sent just before the tablet moved)
    // refreshes data but does not ring.
    final otherSite = m.siteId.isNotEmpty && session.site != null && session.site!.id != m.siteId;
    final ring = inUse && !otherSite;
    try {
      switch (m.kind) {
        case PushKind.sos:
          await Future.wait([if (ring) _show(m), session.refreshSos()]);
        case PushKind.sosAck:
          await session.refreshSos();
          await _cancel(sosNotificationId(m.refId));
        case PushKind.patrolNotStarted:
          if (ring) await _show(m);
          if (session.guard != null) await session.refreshGuardStatus();
        case PushKind.deviceStatus:
          await session.refreshDevice();
        case PushKind.sync:
          await session.syncReference();
        case PushKind.unknown:
          break;
      }
    } catch (e, st) {
      onError?.call(e, st);
    }
  }

  Future<void> _show(PushMessage m) async {
    final note = noteFor(m, patrolTitle: S(session.locale.languageCode).t('push.patrolTitle'));
    if (note == null) return;
    try {
      await _notifier.show(note);
    } catch (e, st) {
      onError?.call(e, st);
    }
  }

  Future<void> _cancel(int id) async {
    try {
      await _notifier.cancel(id);
    } catch (_) {}
  }

  void _open(String payload) => onOpen?.call(payload);

  /// Catches up on what arrived while the app was in the background.
  @visibleForTesting
  Future<void> onResumed() async {
    await _consumeFlags();
    final t = _transport;
    if (!_ready || t == null) return;
    final before = permission;
    permission = await _permission(t.permission);
    if (permission != before) {
      _notify();
      if (permission == PushPermission.granted && _registeredToken == null) unawaited(register());
    }
  }

  Future<void> _consumeFlags() async {
    final phase = session.phase;
    if (phase == Phase.setup || phase == Phase.login || phase == Phase.booting) return;
    Set<String> f;
    try {
      f = await _flags.take();
    } catch (_) {
      return;
    }
    if (f.isEmpty) return;
    try {
      if (f.contains(PushFlags.device)) await session.refreshDevice();
      if (f.contains(PushFlags.sync)) await session.syncReference();
      if (f.contains(PushFlags.sos)) await session.refreshSos();
      if (f.contains(PushFlags.status) && session.guard != null) await session.refreshGuardStatus();
    } catch (e, st) {
      onError?.call(e, st);
    }
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) unawaited(onResumed());
  }

  void _notify() {
    if (!_disposed) notifyListeners();
  }

  @override
  void dispose() {
    _disposed = true;
    _retry?.cancel();
    for (final s in _subs) {
      unawaited(s.cancel());
    }
    if (_started) WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }
}
