import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Plus, Pencil } from 'lucide-react'
import { Button } from '../../components/ui/Button'
import { Toolbar } from '../../components/ui/States'
import { Table, type Column } from '../../components/ui/Table'
import { useSite } from '../../context/site'
import { useI18n } from '../../i18n/context'
import { SiteModal } from './SiteModal'
import type { Site } from '../../types'

export function SitesTab() {
  const { t } = useI18n()
  const { sites, isLoading, error } = useSite()
  const [params] = useSearchParams()
  const [editing, setEditing] = useState<Site | 'new' | null>(() => (params.get('new') === '1' ? 'new' : null))

  const columns: Column<Site>[] = [
    { key: 'name', header: t('site.name'), cell: (s) => <span className="font-medium">{s.name}</span> },
    { key: 'client', header: t('site.clientId'), cell: (s) => s.client_id ?? '—' },
    { key: 'address', header: t('site.address'), cell: (s) => s.address ?? '—' },
    { key: 'tz', header: t('common.timezone'), cell: (s) => s.timezone },
    {
      key: 'cfg',
      header: t('site.clockWindow'),
      numeric: true,
      cell: (s) => s.config_json?.clock_window_minutes ?? '—',
    },
    {
      key: 'actions',
      header: <span className="sr-only">{t('common.actions')}</span>,
      cell: (s) => (
        <Button size="sm" variant="secondary" onClick={() => setEditing(s)} aria-label={`${t('common.edit')} — ${s.name}`}>
          <Pencil className="h-3.5 w-3.5" aria-hidden="true" />
          {t('common.edit')}
        </Button>
      ),
    },
  ]

  return (
    <>
      <Toolbar>
        <Button className="ml-auto" onClick={() => setEditing('new')}>
          <Plus className="h-4 w-4" aria-hidden="true" />
          {t('site.new')}
        </Button>
      </Toolbar>
      <Table columns={columns} rows={sites} rowKey={(s) => s.id} isLoading={isLoading} error={error} empty={t('site.noneHint')} />
      {editing && <SiteModal site={editing === 'new' ? null : editing} onClose={() => setEditing(null)} />}
    </>
  )
}
