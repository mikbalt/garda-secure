import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Plus, Pencil, KeyRound, CheckCircle2, XCircle } from 'lucide-react'
import { apiGet } from '../api/client'
import { RequireSite } from '../components/RequireSite'
import { Badge, StatusBadge } from '../components/ui/Badge'
import { Button } from '../components/ui/Button'
import { Checkbox } from '../components/ui/Form'
import { Notice, PageHeader, Toolbar } from '../components/ui/States'
import { Table, type Column } from '../components/ui/Table'
import { useSite } from '../context/site'
import { useAuth } from '../hooks/useAuth'
import { useT } from '../i18n/context'
import { GuardModal } from './guards/GuardModal'
import { PinModal } from './guards/PinModal'
import type { Guard, Site } from '../types'

function GuardsContent({ site }: { site: Site }) {
  const t = useT()
  const { can } = useAuth()
  const { sites } = useSite()
  const canEdit = can('editGuards')
  const [allSites, setAllSites] = useState(false)
  const [editing, setEditing] = useState<Guard | 'new' | null>(null)
  const [pinFor, setPinFor] = useState<Guard | null>(null)

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['guards', allSites ? 'all' : site.id],
    queryFn: () => apiGet<Guard[]>('/guards', allSites ? {} : { site_id: site.id }),
  })

  const siteName = (id?: string) => (id ? sites.find((s) => s.id === id)?.name ?? '—' : t('common.unassigned'))

  const columns: Column<Guard>[] = [
    { key: 'name', header: t('common.name'), cell: (g) => <span className="font-medium">{g.name}</span> },
    { key: 'emp', header: t('grd.employeeId'), cell: (g) => g.employee_id ?? '—' },
    ...(allSites ? [{ key: 'site', header: t('common.site'), cell: (g: Guard) => siteName(g.site_id) }] : []),
    { key: 'role', header: t('grd.role'), cell: (g) => <Badge tone={g.role === 'supervisor' ? 'purple' : 'gray'}>{t(g.role === 'supervisor' ? 'role.supervisor' : 'role.guard')}</Badge> },
    { key: 'status', header: t('common.status'), cell: (g) => <StatusBadge status={g.status} /> },
    {
      key: 'pin',
      header: t('grd.pin'),
      cell: (g) =>
        g.has_pin ? (
          <span className="inline-flex items-center gap-1 text-green-800">
            <CheckCircle2 className="h-4 w-4" aria-hidden="true" />
            {t('grd.hasPin')}
          </span>
        ) : (
          <span className="inline-flex items-center gap-1 text-red-800">
            <XCircle className="h-4 w-4" aria-hidden="true" />
            {t('grd.noPin')}
          </span>
        ),
    },
  ]
  if (canEdit) {
    columns.push({
      key: 'actions',
      header: <span className="sr-only">{t('common.actions')}</span>,
      cell: (g) => (
        <span className="flex gap-1">
          <Button size="sm" variant="secondary" onClick={() => setEditing(g)} aria-label={`${t('common.edit')} — ${g.name}`}>
            <Pencil className="h-3.5 w-3.5" aria-hidden="true" />
            {t('common.edit')}
          </Button>
          <Button size="sm" variant="secondary" onClick={() => setPinFor(g)} aria-label={`${t('grd.setPin')} — ${g.name}`}>
            <KeyRound className="h-3.5 w-3.5" aria-hidden="true" />
            {t('grd.setPin')}
          </Button>
        </span>
      ),
    })
  }

  return (
    <>
      {!canEdit && <Notice>{t('grd.readOnly')}</Notice>}
      <Toolbar>
        <Checkbox label={t('grd.showAll')} checked={allSites} onChange={(e) => setAllSites(e.target.checked)} />
        {canEdit && (
          <Button className="ml-auto" onClick={() => setEditing('new')}>
            <Plus className="h-4 w-4" aria-hidden="true" />
            {t('grd.new')}
          </Button>
        )}
      </Toolbar>
      <Table columns={columns} rows={data} rowKey={(g) => g.id} isLoading={isLoading} error={error} onRetry={() => refetch()} empty={t('grd.empty')} />
      {editing && <GuardModal guard={editing === 'new' ? null : editing} defaultSiteId={site.id} onClose={() => setEditing(null)} />}
      {pinFor && <PinModal guard={pinFor} onClose={() => setPinFor(null)} />}
    </>
  )
}

export default function Guards() {
  const t = useT()
  return (
    <div>
      <PageHeader title={t('grd.title')} />
      <RequireSite>{(site) => <GuardsContent site={site} />}</RequireSite>
    </div>
  )
}
