import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Plus, Pencil, QrCode } from 'lucide-react'
import { apiGet } from '../../api/client'
import { StatusBadge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Toolbar } from '../../components/ui/States'
import { Table, type Column } from '../../components/ui/Table'
import { useI18n } from '../../i18n/context'
import { RouteEditor } from './RouteEditor'
import { QrSheet } from './QrSheet'
import type { PatrolRoute, Site } from '../../types'

export function RoutesTab({ site, canEdit }: { site: Site; canEdit: boolean }) {
  const { t } = useI18n()
  const [editing, setEditing] = useState<PatrolRoute | 'new' | null>(null)
  const [qrFor, setQrFor] = useState<PatrolRoute | null>(null)

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['patrol-routes', site.id],
    queryFn: () => apiGet<PatrolRoute[]>('/patrol/routes', { site_id: site.id }),
  })

  const columns: Column<PatrolRoute>[] = [
    { key: 'name', header: t('pat.routeName'), cell: (r) => <span className="font-medium">{r.name}</span> },
    {
      key: 'cps',
      header: t('pat.checkpoints'),
      cell: (r) => (
        <div>
          <div>{t('pat.cpCount', { n: r.checkpoints.length })}</div>
          <div className="text-xs text-gray-600">
            {[...r.checkpoints]
              .sort((a, b) => a.order_index - b.order_index)
              .map((c) => c.name)
              .join(' → ')}
          </div>
        </div>
      ),
    },
    { key: 'active', header: t('common.status'), cell: (r) => <StatusBadge status={r.active ? 'active' : 'inactive'} /> },
    {
      key: 'actions',
      header: <span className="sr-only">{t('common.actions')}</span>,
      cell: (r) => (
        <span className="flex gap-1">
          {canEdit && (
            <Button size="sm" variant="secondary" onClick={() => setEditing(r)} aria-label={`${t('common.edit')} — ${r.name}`}>
              <Pencil className="h-3.5 w-3.5" aria-hidden="true" />
              {t('common.edit')}
            </Button>
          )}
          <Button size="sm" variant="secondary" onClick={() => setQrFor(r)} aria-label={`${t('pat.printQr')} — ${r.name}`}>
            <QrCode className="h-3.5 w-3.5" aria-hidden="true" />
            {t('pat.printQr')}
          </Button>
        </span>
      ),
    },
  ]

  return (
    <>
      {canEdit && (
        <Toolbar>
          <Button className="ml-auto" onClick={() => setEditing('new')}>
            <Plus className="h-4 w-4" aria-hidden="true" />
            {t('pat.newRoute')}
          </Button>
        </Toolbar>
      )}
      <Table columns={columns} rows={data} rowKey={(r) => r.id} isLoading={isLoading} error={error} onRetry={() => refetch()} empty={t('pat.noRoutes')} />
      {editing && <RouteEditor site={site} route={editing === 'new' ? null : editing} onClose={() => setEditing(null)} />}
      {qrFor && <QrSheet route={qrFor} onClose={() => setQrFor(null)} />}
    </>
  )
}
