import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:intl/intl.dart';
import 'package:securepatrol/core/database/local_db.dart';
import 'package:securepatrol/core/database/sync_service.dart';
import 'package:securepatrol/core/services/clock.dart';
import 'package:securepatrol/core/services/jpeg_meta.dart';
import 'package:securepatrol/core/services/photo_service.dart';
import 'package:securepatrol/core/storage/field_cipher.dart';
import 'package:securepatrol/core/storage/secure_store.dart';
import 'package:securepatrol/l10n/strings.dart';
import 'package:securepatrol/shared/models/models.dart';
import 'package:sqflite_common_ffi/sqflite_ffi.dart';

import 'support/fake_api.dart';

void main() {
  sqfliteFfiInit();

  group('field encryption (MOB-6)', () {
    test('seal/open round trip, random nonce, tamper detection, legacy passthrough', () async {
      final store = MemorySecureStore();
      final c = await FieldCipher.load(store);
      expect(store.data[SecureKeys.dbKey], isNotNull, reason: 'key kept in secure storage');
      final again = await FieldCipher.load(store);
      final a = c.seal('rahasia'), b = c.seal('rahasia');
      expect(a, isNot(b));
      expect(again.open(a), 'rahasia', reason: 'same key after restart');
      expect(c.open('{"plain":true}'), '{"plain":true}');
      final bytes = base64Decode(a.substring(3));
      bytes[bytes.length - 1] ^= 1;
      expect(() => c.open('e1:${base64Encode(bytes)}'), throwsA(isA<CipherError>()));
      expect(() => FieldCipher(List.filled(32, 7)).open(a), throwsA(isA<CipherError>()));
    });

    test('kv values and outbox bodies are encrypted at rest', () async {
      final db = await LocalDb.open(factory: databaseFactoryFfi, path: inMemoryDatabasePath);
      await db.put(CacheKeys.roster, [
        {'id': 'g', 'name': 'Budi', 'pin_hash': r'$2a$12$secret'}
      ]);
      final o = Outbox(db, FakeApi(), connectivity: const Stream.empty());
      await o.enqueue(OutboxOp(id: '1', kind: 'visitor', method: 'POST', path: '/visitors', guardId: 'g', guardToken: 'tok-123', label: 'Siti Aminah', body: const {'visitor_name': 'Siti Aminah', 'id_number': '3201'}), send: false);
      final raw = jsonEncode([...await db.db.query('kv'), ...await db.db.query('outbox')]);
      for (final secret in ['Budi', r'$2a$12$secret', 'Siti Aminah', '3201', 'tok-123']) {
        expect(raw.contains(secret), isFalse, reason: '$secret must not be stored in clear');
      }
      expect((await db.getList(CacheKeys.roster)).single['name'], 'Budi');
      final op = (await o.list()).single;
      expect(op.body['visitor_name'], 'Siti Aminah');
      expect(op.guardToken, 'tok-123');
      expect(op.label, 'Siti Aminah');
      o.dispose();
      await db.db.close();
    });

    test('a version-1 plaintext database is migrated (schema + encryption)', () async {
      final dir = await Directory.systemTemp.createTemp('sp_db');
      final path = '${dir.path}/v1.db';
      final v1 = await databaseFactoryFfi.openDatabase(path,
          options: OpenDatabaseOptions(
            version: 1,
            onCreate: (d, _) async {
              await d.execute(
                  "CREATE TABLE outbox (id TEXT PRIMARY KEY, kind TEXT NOT NULL, method TEXT NOT NULL, path TEXT NOT NULL, body TEXT NOT NULL, guard_id TEXT, photos TEXT NOT NULL DEFAULT '[]', priority INTEGER NOT NULL DEFAULT 1, label TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'pending', attempts INTEGER NOT NULL DEFAULT 0, last_error TEXT, next_at TEXT NOT NULL, created_at TEXT NOT NULL)");
              await d.execute('CREATE INDEX idx_outbox_next ON outbox(status, priority, created_at)');
              await d.execute('CREATE TABLE kv (key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at TEXT NOT NULL)');
            },
          ));
      final now = DateTime.utc(2026, 10, 1).toIso8601String();
      await v1.insert('kv', {'key': 'roster', 'value': '[{"id":"g","name":"Budi","pin_hash":"h"}]', 'updated_at': now});
      for (final id in ['b', 'a']) {
        await v1.insert('outbox', {'id': id, 'kind': 'incident', 'method': 'POST', 'path': '/incidents', 'body': '{"id":"$id"}', 'label': 'L$id', 'next_at': now, 'created_at': now});
      }
      await v1.close();

      final cipher = FieldCipher(List.filled(32, 3));
      final db = await LocalDb.open(factory: databaseFactoryFfi, path: path, cipher: cipher);
      expect((await db.getList('roster')).single['name'], 'Budi');
      final raw = jsonEncode(await db.db.query('kv'));
      expect(raw.contains('Budi'), isFalse);
      final o = Outbox(db, FakeApi(), connectivity: const Stream.empty());
      final ops = await o.list();
      expect(ops.map((x) => x.id), ['b', 'a'], reason: 'insertion order kept via seq');
      expect(ops.first.label, 'Lb');
      o.dispose();
      await db.db.close();

      // Lost key (e.g. secure storage wiped): unreadable data is dropped, not fatal.
      final other = await LocalDb.open(factory: databaseFactoryFfi, path: path, cipher: FieldCipher(List.filled(32, 9)));
      expect(other.keyReset, isTrue);
      expect(await other.get('roster'), isNull);
      await other.db.close();
      await dir.delete(recursive: true);
    });

    test('wipe refuses while the outbox holds items (MOB-2)', () async {
      final db = await LocalDb.open(factory: databaseFactoryFfi, path: inMemoryDatabasePath);
      final o = Outbox(db, FakeApi(), connectivity: const Stream.empty());
      await o.enqueue(OutboxOp(id: '1', kind: 'incident', method: 'POST', path: '/x', body: const {}), send: false);
      expect(db.wipe(), throwsStateError);
      await o.discard('1');
      await db.put('k', 1);
      await db.wipe();
      expect(await db.get('k'), isNull);
      o.dispose();
      await db.db.close();
    });
  });

  group('signed checkpoint QR (D-035)', () {
    const signedId = '6f1c2a3b-4d5e-4f60-8a7b-9c0d1e2f3a4b';
    const route = PatrolRoute(id: 'r', name: 'R', checkpoints: [
      Checkpoint(id: signedId, name: 'Signed', qrSigned: true),
      Checkpoint(id: 'c-legacy', name: 'Legacy', qrCode: 'SP-GATE-1'),
    ]);

    test('payload parsing', () {
      final q = SignedQr.parse(' SP1:$signedId:AbC_-09 ');
      expect(q?.checkpointId, signedId);
      expect(q?.raw, 'SP1:$signedId:AbC_-09');
      expect(SignedQr.parse('SP1:$signedId:'), isNull, reason: 'no signature');
      expect(SignedQr.parse('SP1:$signedId:abc=def'), isNull, reason: 'not base64url');
      expect(SignedQr.parse('SP2:$signedId:abc'), isNull);
      expect(SignedQr.parse('SP1:${signedId.toUpperCase()}:abc'), isNull);
      expect(SignedQr.parse('SP1:not-a-uuid:abc'), isNull);
    });

    test('matching against the active route', () {
      final m = route.matchQr('SP1:$signedId:sig');
      expect(m?.checkpoint.name, 'Signed');
      expect(m?.signed?.raw, 'SP1:$signedId:sig');
      expect(route.matchQr('SP1:00000000-0000-4000-8000-000000000000:sig'), isNull, reason: 'checkpoint of another route');
      final legacy = route.matchQr('SP-GATE-1');
      expect(legacy?.checkpoint.id, 'c-legacy');
      expect(legacy?.signed, isNull);
      expect(route.matchQr(signedId), isNull, reason: 'a bare checkpoint ID is not a QR');
      expect(route.matchQr(''), isNull);
    });

    test('route JSON from /device/routes', () {
      final r = PatrolRoute.fromJson({
        'id': 'r',
        'name': 'R',
        'checkpoints': [
          {'id': signedId, 'name': 'A', 'qr_code': null, 'qr_signed': true, 'order_index': 0},
          {'id': 'b', 'name': 'B', 'qr_code': 'OLD-1', 'qr_signed': false, 'order_index': 1},
        ],
      });
      expect(r.checkpoints.first.qrSigned, isTrue);
      expect(r.checkpoints.first.qrCode, '');
      expect(PatrolRoute.fromJson(r.toJson()).checkpoints.first.qrSigned, isTrue);
      expect(r.byCode('OLD-1')?.id, 'b');
    });
  });

  group('server time (MOB-4) and site time zone (MOB-9)', () {
    tearDown(AppClock.reset);

    test('offset from server_time and from the Date header', () {
      final sent = DateTime.now();
      final recv = sent.add(const Duration(milliseconds: 200));
      final server = sent.toUtc().add(const Duration(minutes: 5, milliseconds: 100));
      AppClock.observeServerTime(server, sent, recv);
      expect(AppClock.offset.inSeconds, 300);
      expect(AppClock.skewed, isTrue);
      expect(AppClock.now().difference(DateTime.now().toUtc()).inSeconds, inInclusiveRange(299, 301));
      // A coarse Date header agreeing within 3 s does not override it.
      AppClock.observeDateHeader(HttpDate.format(server.add(const Duration(milliseconds: 900))), sent, recv);
      expect(AppClock.offset.inSeconds, 300);
      // The device clock was fixed: the header wins.
      AppClock.observeDateHeader(HttpDate.format(DateTime.now().toUtc()), DateTime.now(), DateTime.now());
      expect(AppClock.offset.inSeconds.abs(), lessThanOrEqualTo(1));
      expect(AppClock.skewed, isFalse);
      AppClock.observeDateHeader('garbage', sent, recv);
    });

    test('times are shown in the site zone, whatever the device zone', () {
      final t = DateTime.utc(2026, 10, 12, 1, 30);
      AppClock.setSiteZone('Asia/Jayapura');
      expect(DateFormat.Hm().format(AppClock.site(t)), '10:30');
      AppClock.setSiteZone('Asia/Makassar');
      expect(DateFormat.Hm().format(AppClock.site(t)), '09:30');
      AppClock.setSiteZone('Asia/Jakarta');
      expect(DateFormat.Hm().format(AppClock.site(t)), '08:30');
      AppClock.setSiteZone('Not/AZone');
      expect(AppClock.site(t), t.toLocal());
    });

    test('scheduled patrols from /device/status', () {
      final st = GuardStatus.fromJson({
        'server_time': '2026-10-12T01:00:00Z',
        'patrols_due': [
          {'route_id': 'r2', 'route_name': 'B', 'due_at': '2026-10-12T03:00:00Z', 'status': 'pending'},
          {'route_id': 'r1', 'route_name': 'A', 'due_at': '2026-10-12T00:30:00Z', 'grace_until': '2026-10-12T00:45:00Z', 'status': 'pending'},
          {'route_id': 'r3', 'route_name': 'C', 'due_at': '2026-10-12T00:00:00Z', 'status': 'started'},
          {'bad': true},
        ],
      });
      expect(st.serverTime, DateTime.utc(2026, 10, 12, 1));
      expect(st.patrolsDue, hasLength(3));
      final next = nextDuePatrol(st.patrolsDue)!;
      expect(next.routeName, 'A');
      expect(next.overdue(DateTime.utc(2026, 10, 12, 1)), isTrue);
      expect(nextDuePatrol(st.patrolsDue, routeId: 'r2')!.overdue(DateTime.utc(2026, 10, 12, 1)), isFalse);
      expect(GuardStatus.fromJson(const {}).patrolsDue, isEmpty, reason: 'field may be absent');
    });
  });

  group('photos (MOB-11)', () {
    Uint8List jpeg({int? orientation}) {
      final b = BytesBuilder()..add([0xFF, 0xD8]);
      // APP0 JFIF
      b.add([0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00]);
      // APP1 EXIF, little endian, IFD0 with Orientation + a GPS-ish payload
      final tiff = <int>[0x49, 0x49, 0x2A, 0x00, 0x08, 0x00, 0x00, 0x00, 0x01, 0x00, 0x12, 0x01, 0x03, 0x00, 0x01, 0x00, 0x00, 0x00, orientation ?? 1, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00];
      final payload = [...'Exif'.codeUnits, 0, 0, ...tiff, ...'GPS-LAT-6.2'.codeUnits];
      b.add([0xFF, 0xE1, (payload.length + 2) >> 8, (payload.length + 2) & 0xFF, ...payload]);
      // COM
      final com = 'secret comment'.codeUnits;
      b.add([0xFF, 0xFE, 0x00, com.length + 2, ...com]);
      // DQT (kept), SOS + data + EOI
      b.add([0xFF, 0xDB, 0x00, 0x04, 0x00, 0x01]);
      b.add([0xFF, 0xDA, 0x00, 0x04, 0x01, 0x02, 0x11, 0x22, 0x33, 0xFF, 0xD9]);
      return b.toBytes();
    }

    bool contains(Uint8List hay, List<int> needle) {
      outer:
      for (var i = 0; i + needle.length <= hay.length; i++) {
        for (var j = 0; j < needle.length; j++) {
          if (hay[i + j] != needle[j]) continue outer;
        }
        return true;
      }
      return false;
    }

    test('EXIF/GPS and comments are stripped, orientation kept, image data intact', () {
      final src = jpeg(orientation: 6);
      expect(JpegMeta.orientationOf(src), 6);
      final out = JpegMeta.strip(src);
      expect(contains(out, 'GPS-LAT'.codeUnits), isFalse);
      expect(contains(out, 'secret comment'.codeUnits), isFalse);
      expect(JpegMeta.orientationOf(out), 6);
      expect(contains(out, [0xFF, 0xDA, 0x00, 0x04, 0x01, 0x02, 0x11, 0x22, 0x33, 0xFF, 0xD9]), isTrue);
      expect(out.sublist(0, 4), [0xFF, 0xD8, 0xFF, 0xE0], reason: 'JFIF stays first');
    });

    test('upright photos get no EXIF at all; non-JPEG input is returned unchanged', () {
      final out = JpegMeta.strip(jpeg());
      expect(contains(out, 'Exif'.codeUnits), isFalse);
      final png = Uint8List.fromList([0x89, 0x50, 0x4E, 0x47, 1, 2, 3]);
      expect(JpegMeta.strip(png), same(png));
    });

    test('orphaned photo files are removed, queued ones kept', () async {
      final dir = await Directory.systemTemp.createTemp('sp_photos');
      final keep = File('${dir.path}/keep.jpg')..writeAsStringSync('x');
      final orphan = File('${dir.path}/orphan.jpg')..writeAsStringSync('x');
      expect(await PhotoService.cleanOrphans({keep.path}, dir: dir), 1);
      expect(keep.existsSync(), isTrue);
      expect(orphan.existsSync(), isFalse);
      await dir.delete(recursive: true);
    });
  });

  test('server error codes are translated in both languages', () {
    for (final lang in ['id', 'en']) {
      final s = S(lang);
      expect(s.error('id_conflict'), isNot(contains('id_conflict')));
      expect(s.error('unknown_code', 'raw server text'), 'raw server text');
      expect(s.error(null), s.t('common.error'));
    }
  });
}
