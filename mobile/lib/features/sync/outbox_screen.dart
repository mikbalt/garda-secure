import 'package:flutter/material.dart';
import 'package:intl/intl.dart';
import 'package:provider/provider.dart';

import '../../core/database/sync_service.dart';
import '../../core/services/clock.dart';
import '../../l10n/strings.dart';
import '../../shared/widgets/ui.dart';
import '../../theme/app_theme.dart';
import '../auth/supervisor_gate.dart';

/// What is still on the tablet and why (FR-OFF-01/05): pending items are sent
/// automatically; failed ones were rejected by the server and show the
/// (translated) reason. Discarding needs a supervisor (MOB-2).
class OutboxScreen extends StatefulWidget {
  const OutboxScreen({super.key});

  @override
  State<OutboxScreen> createState() => _OutboxScreenState();
}

class _OutboxScreenState extends State<OutboxScreen> {
  late Future<List<OutboxOp>> _items;
  Outbox? _outbox;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    final o = context.read<Outbox>();
    if (_outbox != o) {
      _outbox?.removeListener(_reload);
      _outbox = o..addListener(_reload);
      _items = o.list();
    }
  }

  void _reload() {
    if (mounted) setState(() => _items = _outbox!.list());
  }

  @override
  void dispose() {
    _outbox?.removeListener(_reload);
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final o = context.watch<Outbox>();
    final last = o.lastSuccess;
    return Scaffold(
      appBar: AppBar(title: Text(s.t('sync.title'))),
      body: FutureBuilder<List<OutboxOp>>(
        future: _items,
        builder: (context, snap) {
          final items = snap.data ?? const <OutboxOp>[];
          return ListView(
            padding: const EdgeInsets.all(20),
            children: [
              Constrained(
                child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
                  SectionCard(
                    child: Row(children: [
                      IconTile(
                        icon: o.online ? Icons.cloud_done_outlined : Icons.cloud_off_outlined,
                        color: o.failed > 0 ? AppColors.danger : (o.pending > 0 ? AppColors.warn : AppColors.ok),
                      ),
                      const SizedBox(width: 14),
                      Expanded(
                        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                          Text(
                            o.pending + o.failed == 0
                                ? s.t('sync.allSent')
                                : [if (o.pending > 0) s.t('sync.pending', {'n': o.pending}), if (o.failed > 0) s.t('sync.failed', {'n': o.failed})].join(' · '),
                            style: Theme.of(context).textTheme.titleMedium,
                          ),
                          const SizedBox(height: 2),
                          Text(
                            last == null ? s.t('sync.never') : s.t('sync.lastSync', {'time': DateFormat('d MMM HH:mm', s.lang).format(AppClock.site(last))}),
                            style: Theme.of(context).textTheme.bodySmall,
                          ),
                        ]),
                      ),
                      if (o.pending > 0) FilledButton.tonal(onPressed: o.flush, child: Text(s.t('sync.sendNow'))),
                    ]),
                  ),
                  if (o.pauseCode != null) ...[
                    const SizedBox(height: 12),
                    SectionCard(
                      color: AppColors.warnBg,
                      borderColor: const Color(0xFFFDE68A),
                      child: Row(children: [
                        const Icon(Icons.pause_circle_outline, color: AppColors.warn),
                        const SizedBox(width: 12),
                        Expanded(child: Text(s.t('sync.paused', {'reason': s.error(o.pauseCode)}), style: const TextStyle(fontWeight: FontWeight.w500))),
                      ]),
                    ),
                  ],
                  const SizedBox(height: 8),
                  Padding(padding: const EdgeInsets.symmetric(vertical: 8), child: Text(s.t('sync.subtitle'), style: Theme.of(context).textTheme.bodySmall)),
                  for (final op in items) Padding(padding: const EdgeInsets.only(bottom: 10), child: _OpTile(op: op)),
                ]),
              ),
            ],
          );
        },
      ),
    );
  }
}

class _OpTile extends StatelessWidget {
  const _OpTile({required this.op});

  final OutboxOp op;

  /// Never silently: confirm, then a supervisor PIN or the device password.
  Future<void> _discard(BuildContext context, Outbox o) async {
    final s = S.of(context);
    final sure = await showDialog<bool>(
      context: context,
      builder: (c) => AlertDialog(
        content: Text(s.t('sync.discardConfirm')),
        actions: [
          TextButton(onPressed: () => Navigator.pop(c, false), child: Text(s.t('common.cancel'))),
          FilledButton(style: FilledButton.styleFrom(backgroundColor: AppColors.danger), onPressed: () => Navigator.pop(c, true), child: Text(s.t('sync.discard'))),
        ],
      ),
    );
    if (sure != true || !context.mounted) return;
    if (!await requireSupervisor(context)) return;
    await o.discard(op.id);
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final failed = op.status == 'failed';
    final o = context.read<Outbox>();
    return SectionCard(
      borderColor: failed ? const Color(0xFFFECACA) : null,
      padding: const EdgeInsets.fromLTRB(16, 14, 8, 14),
      child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Icon(failed ? Icons.error_outline : (op.kind == 'sos' ? Icons.sos_rounded : Icons.schedule_send_outlined), color: failed ? AppColors.danger : (op.kind == 'sos' ? AppColors.sos : AppColors.warn)),
        const SizedBox(width: 12),
        Expanded(
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Text('${s.t('sync.k.${op.kind}')}${op.label.isNotEmpty ? ' · ${op.label}' : ''}', style: const TextStyle(fontWeight: FontWeight.w600)),
            const SizedBox(height: 2),
            Text(
              [
                DateFormat('d MMM HH:mm', s.lang).format(AppClock.site(op.createdAt)),
                if (op.photos.isNotEmpty) '${op.photos.length} ${s.t('common.photo').toLowerCase()}',
                if (op.guardId != null && op.guardToken == null) s.t('sync.unverified'),
              ].join(' · '),
              style: Theme.of(context).textTheme.bodySmall,
            ),
            if (op.lastError != null || op.lastCode != null) ...[
              const SizedBox(height: 4),
              Text(s.error(op.lastCode, op.lastError), style: TextStyle(fontSize: 13, color: failed ? AppColors.danger : AppColors.textMuted)),
            ],
          ]),
        ),
        if (failed)
          PopupMenuButton<String>(
            onSelected: (v) => v == 'retry' ? o.retry(op.id) : _discard(context, o),
            itemBuilder: (_) => [
              PopupMenuItem(value: 'retry', child: Text(s.t('common.retry'))),
              PopupMenuItem(value: 'discard', child: Text(s.t('sync.discard'))),
            ],
          ),
      ]),
    );
  }
}
