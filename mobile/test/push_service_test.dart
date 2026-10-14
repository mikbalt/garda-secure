import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:securepatrol/core/api/api_client.dart';
import 'package:securepatrol/core/auth/auth_provider.dart';
import 'package:securepatrol/core/services/health.dart';
import 'package:securepatrol/core/services/push_service.dart';
import 'package:securepatrol/features/device/device_info_screen.dart';
import 'package:securepatrol/shared/models/models.dart';

import 'support/fake_api.dart';
import 'support/harness.dart';

const _fcm = PushConfig(apiKey: 'key', appId: '1:123:android:abc', projectId: 'garda-test', senderId: '123');

class FakeTransport implements PushTransport {
  PushPermission perm = PushPermission.granted;
  PushPermission afterRequest = PushPermission.granted;
  String? tokenValue = 'tok-1';
  int inits = 0;
  int permissionRequests = 0;
  int deletes = 0;
  final messagesC = StreamController<Map<String, dynamic>>.broadcast();
  final refreshC = StreamController<String>.broadcast();
  final openedC = StreamController<Map<String, dynamic>>.broadcast();

  @override
  Future<void> init(PushConfig config) async => inits++;

  @override
  Future<PushPermission> permission() async => perm;

  @override
  Future<PushPermission> requestPermission() async {
    permissionRequests++;
    return perm = afterRequest;
  }

  @override
  Future<String?> token() async => tokenValue;

  @override
  Stream<String> get tokenRefresh => refreshC.stream;

  @override
  Stream<Map<String, dynamic>> get messages => messagesC.stream;

  @override
  Stream<Map<String, dynamic>> get opened => openedC.stream;

  @override
  Future<Map<String, dynamic>?> initialMessage() async => null;

  @override
  Future<void> deleteToken() async {
    deletes++;
    tokenValue = 'tok-new';
  }
}

class FakeNotifier implements PushNotifier {
  final shown = <PushNote>[];
  final cancelled = <int>[];
  void Function(String payload)? onTap;

  @override
  Future<void> init({void Function(String payload)? onTap, Map<String, String> channelDescriptions = const {}}) async => this.onTap = onTap;

  @override
  Future<void> show(PushNote note) async => shown.add(note);

  @override
  Future<void> cancel(int id) async => cancelled.add(id);

  @override
  Future<String?> launchPayload() async => null;
}

/// Lets stream listeners and the futures they start run (the test database
/// works on another isolate, so this waits in real time).
Future<void> settle() => Future<void>.delayed(const Duration(milliseconds: 60));

/// Waits until [done] (or fails after 3 s).
Future<void> until(bool Function() done) async {
  final end = DateTime.now().add(const Duration(seconds: 3));
  while (!done()) {
    if (DateTime.now().isAfter(end)) fail('condition not reached');
    await Future<void>.delayed(const Duration(milliseconds: 10));
  }
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  late Harness h;
  late FakeTransport fcm;
  late FakeNotifier notes;
  late MemoryPushFlagStore flags;
  late PushService push;
  var deviceStatus = 'active';

  /// A server that answers every tablet call the push flows trigger.
  Future<Object?> server(Call c) async {
    switch (c.path) {
      case '/device/sos':
        return [
          {'id': 'sos-9', 'guard_name': 'Agus Pratama', 'fired_at': DateTime.now().toUtc().toIso8601String()}
        ];
      case '/device/me':
        return {
          'device': {'id': 'dev-1', 'name': 'Tablet Pos Utama', 'status': deviceStatus, 'site_id': 'site-1'},
          'site': {'id': 'site-1', 'name': 'Gudang Cikarang'},
        };
      case '/device/roster':
        return {'guards': h.session.roster.map((g) => g.toJson()).toList()};
      case '/device/routes':
        return <Object?>[];
      default:
        return null; // 204 / {}
    }
  }

  List<Call> calls(String method, String path) => h.api.calls.where((c) => c.method == method && c.path == path).toList();

  PushService make({PushConfig config = _fcm, List<Duration>? retry}) => PushService(
        session: h.session,
        config: config,
        transport: fcm,
        notifier: notes,
        flags: flags,
        retryDelays: retry ?? const [Duration(hours: 1)],
        platform: 'android',
      )..attach();

  setUp(() async {
    h = await Harness.create();
    h.seedSite();
    deviceStatus = 'active';
    h.api.handler = server;
    fcm = FakeTransport();
    notes = FakeNotifier();
    flags = MemoryPushFlagStore();
    push = make();
  });

  tearDown(() async {
    push.dispose();
    await h.dispose();
  });

  group('token registration', () {
    test('start registers the token: PUT /device/push-token', () async {
      await push.start();
      final put = calls('PUT', '/device/push-token').single;
      expect(put.json, {'token': 'tok-1', 'platform': 'android'});
      expect(put.guardId, isNull, reason: 'no X-Guard-ID');
      expect(push.state, PushState.active);
      expect(push.healthy, isTrue);
      expect(push.beaconStatus, 'active');
    });

    test('a refreshed token is sent again', () async {
      await push.start();
      fcm.refreshC.add('tok-2');
      await until(() => push.registeredToken == 'tok-2');
      expect(calls('PUT', '/device/push-token').map((c) => c.json['token']), ['tok-1', 'tok-2']);
      expect(push.registeredToken, 'tok-2');
    });

    test('pending device: token sent, permission asked, sent again after approval', () async {
      h.session.device = const Device(id: 'dev-1', name: 'Tablet Pos Utama', status: DeviceStatusKind.pending);
      h.session.guard = null;
      h.session.phase = Phase.pending;
      fcm.perm = PushPermission.notDetermined;
      await push.start();
      expect(calls('PUT', '/device/push-token'), hasLength(1), reason: 'pending devices may register (contract)');
      expect(fcm.permissionRequests, 1, reason: 'asked while the operator is at the pending screen');

      await h.session.refreshDevice(); // admin approved
      expect(h.session.phase, Phase.locked);
      await until(() => calls('PUT', '/device/push-token').length == 2);
    });

    test('re-registration of the device sends the token', () async {
      await push.start();
      h.session.onRegistered!();
      await until(() => calls('PUT', '/device/push-token').length == 2);
    });

    test('offline: retried until the server has the token', () async {
      push.dispose();
      push = make(retry: const [Duration(milliseconds: 20)]);
      var fail = true;
      h.api.handler = (c) async {
        if (c.path == '/device/push-token' && fail) throw const ApiException(network: true, message: 'offline');
        return server(c);
      };
      await push.start();
      expect(push.state, PushState.tokenPending);
      expect(push.beaconStatus, 'error');
      expect(push.healthy, isFalse);
      fail = false;
      await Future<void>.delayed(const Duration(milliseconds: 80));
      expect(push.state, PushState.active);
      expect(calls('PUT', '/device/push-token').length, greaterThanOrEqualTo(2));
    });

    test('device sign-out deletes the token on the server and in FCM', () async {
      await push.start();
      await h.session.signOutDevice();
      expect(calls('DELETE', '/device/push-token'), hasLength(1));
      expect(fcm.deletes, 1);
      expect(push.registeredToken, isNull);
      expect(h.session.phase, Phase.login);
    });

    test('sign-out goes on when the DELETE fails', () async {
      await push.start();
      h.api.handler = (c) async => throw const ApiException(status: 500, message: 'boom');
      await h.session.signOutDevice();
      expect(h.session.phase, Phase.login);
    });

    test('not configured: nothing is initialised or sent', () async {
      push.dispose();
      push = make(config: const PushConfig());
      await push.start();
      expect(fcm.inits, 0);
      expect(calls('PUT', '/device/push-token'), isEmpty);
      expect(push.state, PushState.unconfigured);
      expect(push.beaconStatus, 'unconfigured');
    });

    test('permission denied: not healthy', () async {
      fcm.perm = PushPermission.denied;
      await push.start();
      expect(push.state, PushState.denied);
      expect(push.beaconStatus, 'denied');
      expect(push.healthy, isFalse);
      fcm.afterRequest = PushPermission.granted;
      expect(await push.requestPermission(), PushPermission.granted);
      expect(push.state, PushState.active);
    });
  });

  group('messages', () {
    Map<String, dynamic> msg(String type, {String ref = 'sos-9', String site = 'site-1'}) =>
        {'type': type, 'site_id': site, 'ref_id': ref, 'title': 'SOS di Gudang Cikarang', 'body': 'Agus Pratama · Pos Utama'};

    test('sos: refreshes SOS alerts and rings', () async {
      await push.start();
      h.api.calls.clear();
      fcm.messagesC.add(msg('sos'));
      await until(() => h.session.sosAlerts.isNotEmpty && notes.shown.isNotEmpty);
      expect(h.api.paths, contains('/device/sos'));
      expect(h.session.sosAlerts.map((a) => a.id), ['sos-9']);
      final n = notes.shown.single;
      expect(n.channel, PushChannels.sos);
      expect(n.title, 'SOS · Agus Pratama · Pos Utama');
      expect(n.id, sosNotificationId('sos-9'));
      expect(n.payload, 'sos:sos-9');
    });

    test('sos of another site refreshes but does not ring', () async {
      await push.start();
      fcm.messagesC.add(msg('sos', site: 'site-2'));
      await until(() => h.session.sosAlerts.isNotEmpty);
      await settle();
      expect(h.api.paths, contains('/device/sos'));
      expect(notes.shown, isEmpty);
    });

    test('sos_ack: refreshes and cancels the SOS notification', () async {
      await push.start();
      fcm.messagesC.add(msg('sos_ack'));
      await until(() => notes.cancelled.isNotEmpty);
      expect(h.api.paths, contains('/device/sos'));
      expect(notes.cancelled, [sosNotificationId('sos-9')]);
      expect(notes.shown, isEmpty);
    });

    test('patrol_not_started: guard status refreshed, normal notification to home', () async {
      await push.start();
      h.api.calls.clear();
      fcm.messagesC.add({'type': 'patrol_not_started', 'site_id': 'site-1', 'ref_id': 'exp-1', 'title': '', 'body': 'Patroli Malam Perimeter 22:00'});
      await until(() => notes.shown.isNotEmpty && h.api.paths.contains('/device/status'));
      expect(h.api.paths, contains('/device/status'));
      final n = notes.shown.single;
      expect(n.channel, PushChannels.patrol);
      expect(n.title, 'Patroli belum dimulai');
      expect(n.payload, 'home');
    });

    test('device_status: device re-read at once', () async {
      await push.start();
      h.api.calls.clear();
      fcm.messagesC.add({'type': 'device_status', 'ref_id': 'dev-1'});
      await until(() => h.api.paths.contains('/device/me'));
      expect(h.api.paths, contains('/device/me'));
      expect(notes.shown, isEmpty);
    });

    test('device_status on a pending tablet moves it on without waiting for the poll', () async {
      h.session.device = const Device(id: 'dev-1', name: 'Tablet Pos Utama', status: DeviceStatusKind.pending);
      h.session.guard = null;
      h.session.phase = Phase.pending;
      await push.start();
      fcm.messagesC.add({'type': 'device_status'});
      await until(() => h.session.phase == Phase.locked);
    });

    test('sync: roster and routes synced', () async {
      await push.start();
      h.api.calls.clear();
      fcm.messagesC.add({'type': 'sync'});
      await until(() => h.api.paths.contains('/device/routes'));
      expect(h.api.paths, containsAll(['/device/roster', '/device/routes']));
    });

    test('unknown types are ignored', () async {
      await push.start();
      h.api.calls.clear();
      fcm.messagesC.add({'type': 'marketing', 'title': 'x'});
      fcm.messagesC.add(const {});
      await settle();
      expect(h.api.calls, isEmpty);
      expect(notes.shown, isEmpty);
      expect(notes.cancelled, isEmpty);
    });

    test('messages are ignored before the device is signed in', () async {
      h.session.phase = Phase.setup;
      await push.start();
      fcm.messagesC.add(msg('sos'));
      await settle();
      expect(notes.shown, isEmpty);
    });

    test('flags from the background handler are applied on resume', () async {
      await push.start();
      h.api.calls.clear();
      await flags.add(PushFlags.sos);
      await flags.add(PushFlags.device);
      await push.onResumed();
      expect(h.api.paths, containsAll(['/device/me', '/device/sos']));
      expect(flags.flags, isEmpty);
    });

    test('tapping a notification opens its target', () async {
      final opened = <String>[];
      push.onOpen = opened.add;
      await push.start();
      notes.onTap!('sos:sos-9');
      expect(opened, ['sos:sos-9']);
    });
  });

  group('message parsing', () {
    test('types and notification texts', () {
      final m = PushMessage.fromData({'type': 'sos', 'ref_id': 'a', 'title': 'SOS', 'body': 'Budi · Gerbang'});
      expect(m.kind, PushKind.sos);
      expect(m.refreshFlag, PushFlags.sos);
      expect(noteFor(m)!.title, 'SOS · Budi · Gerbang');
      expect(PushMessage.fromData({'type': 'sos'}).kind, PushKind.sos);
      expect(noteFor(PushMessage.fromData({'type': 'sos'}))!.title, 'SOS');
      expect(PushMessage.fromData({'type': 'sync'}).refreshFlag, PushFlags.sync);
      expect(noteFor(PushMessage.fromData({'type': 'device_status'})), isNull);
      expect(PushMessage.fromData({'type': 'nope'}).refreshFlag, isNull);
      expect(sosNotificationId('x'), sosNotificationId('x'));
      expect(sosNotificationId('x'), isNot(sosNotificationId('y')));
      expect(sosNotificationId('x'), inInclusiveRange(0, 0x7fffffff));
    });
  });

  group('SOS polling', () {
    test('15 s / 30 s without push, 60 s with healthy push, never slower', () async {
      h.session.phase = Phase.unlocked;
      expect(h.session.sosPollInterval, const Duration(seconds: 15));
      h.session.phase = Phase.locked;
      expect(h.session.sosPollInterval, const Duration(seconds: 30));

      await push.start();
      expect(push.healthy, isTrue);
      h.session.phase = Phase.unlocked;
      expect(h.session.sosPollInterval, const Duration(seconds: 60));
      h.session.phase = Phase.locked;
      expect(h.session.sosPollInterval, const Duration(seconds: 60));

      h.session.phase = Phase.pending;
      expect(h.session.sosPollInterval, isNull);
    });

    test('back to the fast interval when push is not healthy', () async {
      fcm.perm = PushPermission.denied;
      await push.start();
      h.session.phase = Phase.unlocked;
      expect(h.session.sosPollInterval, const Duration(seconds: 15));
    });

    test('unconfigured push keeps the old intervals', () async {
      push.dispose();
      push = make(config: const PushConfig());
      await push.start();
      h.session.phase = Phase.locked;
      expect(h.session.sosPollInterval, const Duration(seconds: 30));
    });
  });

  test('health beacon reports the push status', () async {
    await push.start();
    final beacon = HealthBeacon(
      api: h.api,
      errors: ErrorLog(h.db),
      active: () => true,
      snapshot: () => HealthSnapshot(outboxPending: 0, outboxFailed: 0, push: push.beaconStatus),
    );
    expect(beacon.payload()['push'], 'active');
  });

  testWidgets('device info shows the push status and asks again', (tester) async {
    fcm.perm = PushPermission.denied;
    fcm.afterRequest = PushPermission.granted;
    await tester.runAsync(push.start);
    await setSurface(tester, tabletSize);
    await tester.pumpWidget(wrap(h, const DeviceInfoScreen(), push: push));
    await tester.pump();
    expect(find.text('Izin ditolak'), findsOneWidget);
    await tester.tap(find.text('Minta izin lagi'));
    await tester.runAsync(() => until(() => push.state == PushState.active));
    await tester.pump();
    expect(fcm.permissionRequests, 1);
    expect(find.text('Aktif'), findsOneWidget);
    await tester.pumpWidget(const SizedBox());
  });
}
