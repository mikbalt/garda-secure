import { useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Eye, ImageOff, ShieldAlert } from 'lucide-react'
import { apiGet } from '../../api/client'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Modal } from '../../components/ui/Modal'
import { ErrorState } from '../../components/ui/States'
import { useI18n } from '../../i18n/context'
import { mediaUrl } from '../../lib/media'
import { formatDateTime } from '../../lib/time'
import type { VisitorDetail, VisitorLog } from '../../types'

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[minmax(0,9rem)_minmax(0,1fr)] gap-3 px-4 py-2.5 text-sm">
      <dt className="text-gray-600">{label}</dt>
      <dd className="min-w-0 break-words text-gray-900">{children}</dd>
    </div>
  )
}

/**
 * Visitor detail (admin/supervisor, D-046): the list shows masked ID numbers;
 * the full number and the photos are fetched only on an explicit action,
 * because every `GET /visitors/:id` is recorded in the audit log.
 */
export function VisitorDrawer({ visitor, tz, onClose }: { visitor: VisitorLog; tz: string; onClose: () => void }) {
  const { t, lang } = useI18n()
  const [revealed, setRevealed] = useState(false)
  const detail = useQuery({
    queryKey: ['visitor-detail', visitor.id],
    queryFn: () => apiGet<VisitorDetail>(`/visitors/${visitor.id}`),
    enabled: revealed,
    // Personal data: do not keep it cached after the drawer closes.
    gcTime: 0,
    staleTime: Infinity,
    retry: false,
  })
  const v = detail.data?.visitor ?? visitor
  const photos = detail.data?.photos ?? []

  return (
    <Modal
      open
      variant="drawer"
      onClose={onClose}
      title={t('acc.vis.title', { name: visitor.visitor_name })}
      footer={
        <Button variant="secondary" onClick={onClose}>
          {t('common.close')}
        </Button>
      }
    >
      <div className="space-y-5">
        <dl className="divide-y divide-gray-100 overflow-hidden rounded-xl border border-gray-200">
          <Row label={t('vis.name')}>{v.visitor_name}</Row>
          <Row label={t('acc.vis.idType')}>{v.id_type || '—'}</Row>
          <Row label={t('acc.vis.idNumber')}>
            <span className="flex flex-wrap items-center gap-2">
              <span className="font-mono tracking-wide">{v.id_number || '—'}</span>
              {detail.data ? <Badge tone="yellow">{t('acc.vis.fullShown')}</Badge> : v.id_number ? <Badge tone="gray">{t('acc.vis.masked')}</Badge> : null}
            </span>
          </Row>
          <Row label={t('vis.plate')}>{v.plate || '—'}</Row>
          <Row label={t('vis.purpose')}>{v.purpose || '—'}</Row>
          <Row label={t('acc.vis.host')}>{v.host_name || '—'}</Row>
          <Row label={t('vis.entry')}>{formatDateTime(v.entry_at, tz, lang)}</Row>
          <Row label={t('vis.exit')}>{v.exit_at ? formatDateTime(v.exit_at, tz, lang) : t('ops.vis.inside')}</Row>
          <Row label={t('ops.vis.loggedBy')}>{v.guard_name || '—'}</Row>
        </dl>

        {!revealed && (
          <div className="rounded-xl border border-amber-200 bg-amber-50 p-3.5 text-sm text-amber-900">
            <p className="flex items-start gap-2">
              <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0 text-amber-600" aria-hidden="true" />
              <span>{t('acc.vis.auditNote')}</span>
            </p>
            <Button className="mt-3" variant="secondary" onClick={() => setRevealed(true)}>
              <Eye className="h-4 w-4" aria-hidden="true" />
              {t('acc.vis.reveal')}
            </Button>
          </div>
        )}
        {revealed && detail.isLoading && <p className="text-sm text-gray-600">{t('app.loading')}</p>}
        {detail.error && <ErrorState error={detail.error} onRetry={() => detail.refetch()} />}

        {detail.data && (
          <section>
            <h3 className="mb-2 text-sm font-semibold text-gray-900">{t('acc.vis.photos')}</h3>
            {photos.length === 0 ? (
              <p className="text-sm text-gray-600">{t('acc.vis.noPhotos')}</p>
            ) : (
              <ul className="grid gap-3 sm:grid-cols-2">
                {photos.map((p) => {
                  const url = p.uploaded ? mediaUrl(p.url) : undefined
                  return (
                    <li key={p.key}>
                      {url ? (
                        <img src={url} alt={t('acc.vis.photoAlt', { name: v.visitor_name })} className="w-full rounded-lg object-contain ring-1 ring-gray-200" />
                      ) : (
                        <div className="flex aspect-video w-full flex-col items-center justify-center gap-2 rounded-lg border border-dashed border-gray-300 bg-gray-50 text-sm text-gray-700">
                          <ImageOff className="h-6 w-6" aria-hidden="true" />
                          {t('common.pendingUpload')}
                        </div>
                      )}
                    </li>
                  )
                })}
              </ul>
            )}
          </section>
        )}
      </div>
    </Modal>
  )
}
