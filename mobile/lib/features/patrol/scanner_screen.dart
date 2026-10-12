import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:mobile_scanner/mobile_scanner.dart';
import 'package:provider/provider.dart';

import '../../core/auth/auth_provider.dart';
import '../../core/services/location_service.dart';
import '../../l10n/strings.dart';
import '../../shared/models/models.dart';
import '../../theme/app_theme.dart';
import '../field_actions.dart';
import '../../core/services/clock.dart';

/// What the scanner returns to the patrol screen.
class ScanResult {
  const ScanResult.scanned(Checkpoint this.checkpoint) : manual = false;
  const ScanResult.manual()
      : checkpoint = null,
        manual = true;

  /// The checkpoint that was recorded.
  final Checkpoint? checkpoint;

  /// The guard asked for the manual (photo) check-in instead.
  final bool manual;
}

/// Full-screen QR scanner for checkpoints. Unknown codes and repeats keep the
/// camera running with a notice; a new checkpoint is recorded and the screen
/// closes with [ScanResult.scanned].
class ScannerScreen extends StatefulWidget {
  const ScannerScreen({super.key});

  @override
  State<ScannerScreen> createState() => _ScannerScreenState();
}

class _ScannerScreenState extends State<ScannerScreen> {
  final _controller = MobileScannerController(formats: const [BarcodeFormat.qrCode]);

  Fix? _fix;
  bool _busy = false;
  String? _lastCode;
  DateTime? _lastAt;
  String? _notice;
  bool _noticeError = false;
  Timer? _noticeTimer;

  @override
  void initState() {
    super.initState();
    // Best-effort GPS in the background: the scan never waits for it.
    context.read<FieldActions>().location.current().then((f) => _fix = f, onError: (_) => null);
  }

  @override
  void dispose() {
    _noticeTimer?.cancel();
    _controller.dispose();
    super.dispose();
  }

  void _show(String text, {required bool error}) {
    _noticeTimer?.cancel();
    setState(() {
      _notice = text;
      _noticeError = error;
    });
    _noticeTimer = Timer(const Duration(seconds: 4), () {
      if (mounted) setState(() => _notice = null);
    });
  }

  Future<void> _onDetect(BarcodeCapture capture) async {
    if (_busy) return;
    String? code;
    for (final b in capture.barcodes) {
      final v = b.rawValue?.trim();
      if (v != null && v.isNotEmpty) {
        code = v;
        break;
      }
    }
    if (code == null) return;
    final now = DateTime.now();
    // The camera reports the same code many times per second.
    if (code == _lastCode && _lastAt != null && now.difference(_lastAt!) < const Duration(seconds: 3)) return;
    _lastCode = code;
    _lastAt = now;

    final actions = context.read<FieldActions>();
    context.read<DeviceSession>().touch();
    final s = S.of(context);
    final p = actions.patrol;
    if (p == null) {
      Navigator.of(context).pop();
      return;
    }
    final cp = p.route.byCode(code);
    if (cp == null) {
      unawaited(HapticFeedback.heavyImpact());
      _show(s.t('pat.unknownQr', {'route': p.route.name}), error: true);
      return;
    }
    if (p.scans.any((x) => x.checkpointId == cp.id)) {
      unawaited(HapticFeedback.lightImpact());
      _show(s.t('pat.alreadyScanned', {'name': cp.name}), error: false);
      return;
    }
    _busy = true;
    try {
      final res = await actions.scan(code, fix: _fix);
      if (!mounted) return;
      if (res == null) {
        _show(s.t('pat.unknownQr', {'route': p.route.name}), error: true);
        return;
      }
      await HapticFeedback.mediumImpact();
      if (mounted) Navigator.of(context).pop(ScanResult.scanned(res));
    } finally {
      _busy = false;
    }
  }

  void _manual() {
    context.read<DeviceSession>().touch();
    Navigator.of(context).pop(const ScanResult.manual());
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final p = context.watch<FieldActions>().patrol;
    final next = p?.progress(AppClock.now()).next?.checkpoint;
    return Scaffold(
      backgroundColor: Colors.black,
      body: Stack(
        fit: StackFit.expand,
        children: [
          MobileScanner(
            controller: _controller,
            onDetect: _onDetect,
            errorBuilder: (context, error, _) => const _CameraError(),
          ),
          ValueListenableBuilder<MobileScannerState>(
            valueListenable: _controller,
            builder: (context, st, _) => st.error != null ? const SizedBox.shrink() : const IgnorePointer(child: CustomPaint(painter: _FramePainter())),
          ),
          SafeArea(
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Column(
                children: [
                  Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      _RoundButton(icon: Icons.close_rounded, tooltip: s.t('common.close'), onTap: () => Navigator.of(context).pop()),
                      const SizedBox(width: 14),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            const SizedBox(height: 4),
                            Text(s.t('pat.scanTitle'), style: const TextStyle(color: Colors.white, fontSize: 20, fontWeight: FontWeight.w600)),
                            const SizedBox(height: 4),
                            Text(s.t('pat2.scanHint'), style: const TextStyle(color: Colors.white70, fontSize: 14)),
                          ],
                        ),
                      ),
                    ],
                  ),
                  if (next != null) ...[
                    const SizedBox(height: 14),
                    Container(
                      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 8),
                      decoration: BoxDecoration(color: Colors.white.withOpacity(0.14), borderRadius: BorderRadius.circular(999)),
                      child: Text(s.t('pat2.nextLabel', {'name': next.name}),
                          style: const TextStyle(color: Colors.white, fontWeight: FontWeight.w600, fontSize: 15)),
                    ),
                  ],
                  const Spacer(),
                  AnimatedSwitcher(
                    duration: const Duration(milliseconds: 200),
                    child: _notice == null
                        ? const SizedBox.shrink()
                        : Container(
                            key: ValueKey(_notice),
                            constraints: const BoxConstraints(maxWidth: 640),
                            margin: const EdgeInsets.only(bottom: 14),
                            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
                            decoration: BoxDecoration(color: _noticeError ? AppColors.danger : AppColors.ink700, borderRadius: BorderRadius.circular(14)),
                            child: Row(
                              children: [
                                Icon(_noticeError ? Icons.error_outline_rounded : Icons.info_outline_rounded, color: Colors.white),
                                const SizedBox(width: 12),
                                Expanded(child: Text(_notice!, style: const TextStyle(color: Colors.white, fontSize: 15, fontWeight: FontWeight.w500))),
                              ],
                            ),
                          ),
                  ),
                  ConstrainedBox(
                    constraints: const BoxConstraints(maxWidth: 640),
                    child: Row(
                      children: [
                        ValueListenableBuilder<MobileScannerState>(
                          valueListenable: _controller,
                          builder: (context, st, _) {
                            final on = st.torchState == TorchState.on;
                            final available = st.torchState != TorchState.unavailable;
                            return _PillButton(
                              icon: on ? Icons.flashlight_on_rounded : Icons.flashlight_off_rounded,
                              label: s.t('pat.torch'),
                              active: on,
                              onTap: available ? () => _controller.toggleTorch() : null,
                            );
                          },
                        ),
                        const SizedBox(width: 12),
                        Expanded(child: _PillButton(icon: Icons.photo_camera_outlined, label: s.t('pat.manual'), onTap: _manual, expand: true)),
                      ],
                    ),
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _CameraError extends StatelessWidget {
  const _CameraError();

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    return ColoredBox(
      color: AppColors.ink,
      child: Center(
        child: Padding(
          padding: const EdgeInsets.all(32),
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 440),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                const Icon(Icons.no_photography_outlined, color: Colors.white70, size: 64),
                const SizedBox(height: 16),
                Text(s.t('pat2.cameraError'), textAlign: TextAlign.center, style: const TextStyle(color: Colors.white, fontSize: 17, height: 1.45)),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _RoundButton extends StatelessWidget {
  const _RoundButton({required this.icon, required this.tooltip, required this.onTap});

  final IconData icon;
  final String tooltip;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) => Material(
        color: Colors.black45,
        shape: const CircleBorder(),
        child: IconButton(
            onPressed: onTap,
            tooltip: tooltip,
            iconSize: 28,
            constraints: const BoxConstraints.tightFor(width: 56, height: 56),
            icon: Icon(icon, color: Colors.white)),
      );
}

class _PillButton extends StatelessWidget {
  const _PillButton({required this.icon, required this.label, required this.onTap, this.active = false, this.expand = false});

  final IconData icon;
  final String label;
  final VoidCallback? onTap;
  final bool active;
  final bool expand;

  @override
  Widget build(BuildContext context) {
    final fg = active ? AppColors.ink : Colors.white;
    return Opacity(
      opacity: onTap == null ? 0.4 : 1,
      child: Material(
        color: active ? Colors.white : Colors.black54,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(14), side: BorderSide(color: Colors.white.withOpacity(0.35))),
        clipBehavior: Clip.antiAlias,
        child: InkWell(
          onTap: onTap,
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 16),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                Icon(icon, color: fg),
                const SizedBox(width: 10),
                if (expand)
                  Flexible(child: Text(label, overflow: TextOverflow.ellipsis, style: TextStyle(color: fg, fontSize: 16, fontWeight: FontWeight.w600)))
                else
                  Text(label, style: TextStyle(color: fg, fontSize: 16, fontWeight: FontWeight.w600)),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

/// Dims everything but a square window and draws its corners.
class _FramePainter extends CustomPainter {
  const _FramePainter();

  @override
  void paint(Canvas canvas, Size size) {
    final side = (size.shortestSide * 0.62).clamp(180.0, 360.0);
    final center = Offset(size.width / 2, size.height * 0.46);
    final rect = Rect.fromCenter(center: center, width: side, height: side);
    final window = RRect.fromRectAndRadius(rect, const Radius.circular(24));
    final scrim = Path.combine(PathOperation.difference, Path()..addRect(Offset.zero & size), Path()..addRRect(window));
    canvas.drawPath(scrim, Paint()..color = Colors.black.withOpacity(0.55));

    final corner = Paint()
      ..color = Colors.white
      ..style = PaintingStyle.stroke
      ..strokeWidth = 6
      ..strokeCap = StrokeCap.round;
    const len = 44.0;
    const r = 24.0;
    final l = rect.left, t = rect.top, rt = rect.right, b = rect.bottom;
    for (final path in [
      Path()
        ..moveTo(l, t + len)
        ..lineTo(l, t + r)
        ..arcToPoint(Offset(l + r, t), radius: const Radius.circular(r))
        ..lineTo(l + len, t),
      Path()
        ..moveTo(rt - len, t)
        ..lineTo(rt - r, t)
        ..arcToPoint(Offset(rt, t + r), radius: const Radius.circular(r))
        ..lineTo(rt, t + len),
      Path()
        ..moveTo(rt, b - len)
        ..lineTo(rt, b - r)
        ..arcToPoint(Offset(rt - r, b), radius: const Radius.circular(r))
        ..lineTo(rt - len, b),
      Path()
        ..moveTo(l + len, b)
        ..lineTo(l + r, b)
        ..arcToPoint(Offset(l, b - r), radius: const Radius.circular(r))
        ..lineTo(l, b - len),
    ]) {
      canvas.drawPath(path, corner);
    }
  }

  @override
  bool shouldRepaint(covariant _FramePainter oldDelegate) => false;
}
