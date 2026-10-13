import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { RefreshCw, TriangleAlert } from 'lucide-react'
import apiClient from '../../api/client'
import { Button } from '../../components/ui/Button'
import { Checkbox, FormError } from '../../components/ui/Form'
import { Modal } from '../../components/ui/Modal'
import { useToast } from '../../components/toast/context'
import { useI18n } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import type { PatrolRoute } from '../../types'

/**
 * Strong confirmation for POST /patrol/routes/:id/regenerate-qr (D-037):
 * every printed code of the route stops matching at once, so the admin must
 * acknowledge the reprint before the button is enabled.
 */
export function RegenerateQrDialog({ route, onClose, onDone }: { route: PatrolRoute; onClose: () => void; onDone: (r: PatrolRoute) => void }) {
  const { t, lang } = useI18n()
  const qc = useQueryClient()
  const toast = useToast()
  const [ack, setAck] = useState(false)
  const n = route.checkpoints.length
  const regen = useMutation({
    mutationFn: async () => (await apiClient.post<PatrolRoute>(`/patrol/routes/${route.id}/regenerate-qr`)).data,
    onSuccess: (r) => {
      toast.success(t('p2.qr.done'))
      void qc.invalidateQueries({ queryKey: ['patrol-routes', route.site_id] })
      onDone(r && Array.isArray(r.checkpoints) ? r : route)
    },
  })
  return (
    <Modal
      open
      onClose={onClose}
      title={t('p2.qr.title', { route: route.name })}
      footer={
        <>
          <Button variant="secondary" onClick={onClose} data-autofocus>
            {t('common.cancel')}
          </Button>
          <Button variant="danger" onClick={() => regen.mutate()} disabled={!ack} loading={regen.isPending}>
            <RefreshCw className="h-4 w-4" aria-hidden="true" />
            {t('p2.qr.confirm', { n })}
          </Button>
        </>
      }
    >
      <div className="space-y-4 text-sm text-gray-700">
        <FormError message={regen.error ? errorText(regen.error, lang) : null} />
        <div className="flex items-start gap-3 rounded-xl border border-red-200 bg-red-50 px-3.5 py-3 text-red-900">
          <TriangleAlert className="mt-0.5 h-5 w-5 shrink-0 text-red-600" aria-hidden="true" />
          <div>
            <p className="font-semibold">{t('p2.qr.warnTitle')}</p>
            <p className="mt-1">{t('p2.qr.warnBody', { n })}</p>
          </div>
        </div>
        <ol className="list-decimal space-y-1 pl-5">
          <li>{t('p2.qr.step1')}</li>
          <li>{t('p2.qr.step2')}</li>
          <li>{t('p2.qr.step3')}</li>
        </ol>
        <Checkbox label={t('p2.qr.ack', { n })} checked={ack} onChange={(e) => setAck(e.target.checked)} />
      </div>
    </Modal>
  )
}
