import 'package:flutter/material.dart';
import 'package:intl/intl.dart';
import 'package:provider/provider.dart';

import '../../core/database/sync_service.dart';
import '../../l10n/strings.dart';
import '../../shared/widgets/ui.dart';
import '../../theme/app_theme.dart';

/// What is still on the tablet and why (FR-OFF-01/05): pending items are sent
/// automatically; failed ones were rejected by the server and show the reason.
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
                            last == null ? s.t('sync.never') : s.t('sync.lastSync', {'time': DateFormat('d MMM HH:mm', s.lang).format(last.toLocal())}),
                            style: Theme.of(context).textTheme.bodySmall,
                          ),
                        ]),
                      ),
                      if (o.pending > 0) FilledButton.tonal(onPressed: o.flush, child: Text(s.t('sync.sendNow'))),
                    ]),
                  ),
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
              [DateFormat('d MMM HH:mm', s.lang).format(op.createdAt.toLocal()), if (op.photos.isNotEmpty) '${op.photos.length} ${s.t('common.photo').toLowerCase()}'].join(' · '),
              style: Theme.of(context).textTheme.bodySmall,
            ),
            if (op.lastError != null) ...[
              const SizedBox(height: 4),
              Text(op.lastError!, style: TextStyle(fontSize: 13, color: failed ? AppColors.danger : AppColors.textMuted)),
            ],
          ]),
        ),
        if (failed)
          PopupMenuButton<String>(
            onSelected: (v) => v == 'retry' ? o.retry(op.id) : o.discard(op.id),
            itemBuilder: (_) => [
              PopupMenuItem(value: 'retry', child: Text(s.t('common.retry'))),
              PopupMenuItem(value: 'discard', child: Text(s.t('sync.discard'))),
            ],
          ),
      ]),
    );
  }
}
