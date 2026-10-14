import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Bell, Send } from 'lucide-react'
import apiClient from '../api/client'
import { Button } from '../components/ui/Button'
import { Notice, PageHeader } from '../components/ui/States'
import { useErrorToast, useToast } from '../components/toast/context'
import { config } from '../config'
import { PUSH_KEYS, usePush } from '../hooks/usePush'
import { useI18n } from '../i18n/context'
import { showLocalTestNotification } from '../lib/push'
import { DeviceList } from './notifications/DeviceList'
import { Preferences } from './notifications/Preferences'
import { ThisDevice } from './notifications/ThisDevice'
import type { PushTestResult } from '../types/push'

/**
 * The user's own push notifications (not site-scoped): this browser on/off,
 * the user's devices, which alerts to receive, and a test push.
 */
export default function Notifications() {
  const { t } = useI18n()
  const qc = useQueryClient()
  const toast = useToast()
  const onError = useErrorToast()
  const push = usePush()

  const test = useMutation({
    mutationFn: async () => (await apiClient.post<PushTestResult>('/push/test')).data,
    onSuccess: (r) => {
      const sent = r?.sent ?? 0
      const failed = r?.failed ?? 0
      if (sent === 0 && failed === 0) toast.info(t('push.test.none'))
      else if (failed > 0) toast.error(t('push.test.partial', { sent, failed }))
      else toast.success(t('push.test.sent', { n: sent }))
      // Demo: no push service delivers it, so show what it looks like here.
      if (config.demo && sent > 0 && push.state === 'on') void showLocalTestNotification(t('push.test.title'), t('push.test.body'))
    },
    onError,
    onSettled: () => qc.invalidateQueries({ queryKey: PUSH_KEYS.subscriptions }),
  })

  const sendTest = () => {
    if (push.rowsQuery.isSuccess && push.rows.length === 0) return toast.info(t('push.test.none'))
    test.mutate()
  }

  return (
    <>
      <PageHeader
        icon={Bell}
        title={t('push.title')}
        subtitle={t('push.subtitle')}
        actions={
          <Button variant="secondary" onClick={sendTest} loading={test.isPending}>
            <Send className="h-4 w-4" aria-hidden="true" />
            {t('push.test')}
          </Button>
        }
      />
      {config.demo && <Notice>{t('push.demoNote')}</Notice>}
      <div className="grid items-start gap-6 lg:grid-cols-5">
        <div className="space-y-6 lg:col-span-2">
          <ThisDevice push={push} />
          <DeviceList push={push} />
        </div>
        <div className="lg:col-span-3">
          <Preferences hasDevice={!push.rowsQuery.isSuccess || push.rows.length > 0} />
        </div>
      </div>
    </>
  )
}
