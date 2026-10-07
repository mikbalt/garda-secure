import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, Pencil, Trash2 } from 'lucide-react'
import apiClient, { apiGet } from '../../api/client'
import { StatusBadge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Notice, Toolbar } from '../../components/ui/States'
import { Table, type Column } from '../../components/ui/Table'
import { useErrorToast, useToast } from '../../components/toast/context'
import { useGuards } from '../../hooks/useGuards'
import { useI18n } from '../../i18n/context'
import { formatDay, weekdayName } from '../../lib/time'
import { TemplateModal } from './TemplateModal'
import type { ShiftTemplate, Site } from '../../types'

export function TemplatesTab({ site, canEdit }: { site: Site; canEdit: boolean }) {
  const { t, lang } = useI18n()
  const qc = useQueryClient()
  const toast = useToast()
  const onError = useErrorToast()
  const { guardName } = useGuards(site.id)
  const [editing, setEditing] = useState<ShiftTemplate | 'new' | null>(null)

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['shift-templates', site.id],
    queryFn: () => apiGet<ShiftTemplate[]>('/shift-templates', { site_id: site.id }),
  })

  const del = useMutation({
    mutationFn: (id: string) => apiClient.delete(`/shift-templates/${id}`),
    onSuccess: () => {
      toast.success(t('sch.templateDeleted'))
      qc.invalidateQueries({ queryKey: ['shift-templates', site.id] })
      qc.invalidateQueries({ queryKey: ['shifts', site.id] })
    },
    onError,
  })

  const columns: Column<ShiftTemplate>[] = [
    { key: 'guard', header: t('common.guard'), cell: (r) => <span className="font-medium">{guardName(r.guard_id)}</span> },
    {
      key: 'time',
      header: t('common.time'),
      cell: (r) => (
        <span className="whitespace-nowrap tabular-nums">
          {r.start_time}–{r.end_time}
          {r.end_time <= r.start_time && <sup className="ml-0.5 text-xs text-gray-600" title={t('sch.endsNextDay')}>+1</sup>}
        </span>
      ),
    },
    {
      key: 'days',
      header: t('sch.weekdays'),
      cell: (r) => (
        <span className="text-xs">
          {[...r.weekdays]
            .sort()
            .map((d) => weekdayName(d, lang))
            .join(', ')}
        </span>
      ),
    },
    {
      key: 'valid',
      header: t('sch.validFrom'),
      cell: (r) => (
        <span className="whitespace-nowrap text-xs">
          {formatDay(r.valid_from, lang, false)} → {r.valid_until ? formatDay(r.valid_until, lang, false) : t('sch.openEnded')}
        </span>
      ),
    },
    { key: 'active', header: t('common.status'), cell: (r) => <StatusBadge status={r.active ? 'active' : 'inactive'} /> },
  ]
  if (canEdit) {
    columns.push({
      key: 'actions',
      header: <span className="sr-only">{t('common.actions')}</span>,
      cell: (r) => (
        <span className="flex gap-1">
          <Button size="sm" variant="ghost" onClick={() => setEditing(r)} aria-label={`${t('common.edit')} — ${guardName(r.guard_id)}`}>
            <Pencil className="h-3.5 w-3.5" aria-hidden="true" />
          </Button>
          <Button
            size="sm"
            variant="ghost"
            className="text-red-700"
            aria-label={`${t('common.delete')} — ${guardName(r.guard_id)}`}
            loading={del.isPending && del.variables === r.id}
            onClick={() => window.confirm(t('sch.confirmDeleteTemplate')) && del.mutate(r.id)}
          >
            <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
          </Button>
        </span>
      ),
    })
  }

  return (
    <>
      <Notice>{t('sch.templateHint')}</Notice>
      {canEdit && (
        <Toolbar>
          <Button className="ml-auto" onClick={() => setEditing('new')}>
            <Plus className="h-4 w-4" aria-hidden="true" />
            {t('sch.newTemplate')}
          </Button>
        </Toolbar>
      )}
      <Table
        columns={columns}
        rows={data}
        rowKey={(r) => r.id}
        isLoading={isLoading}
        error={error}
        onRetry={() => refetch()}
        empty={t('sch.noTemplates')}
      />
      {editing && <TemplateModal site={site} template={editing === 'new' ? null : editing} onClose={() => setEditing(null)} />}
    </>
  )
}
