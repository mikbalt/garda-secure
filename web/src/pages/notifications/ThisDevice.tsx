import { BellOff, BellRing, Monitor, Smartphone } from 'lucide-react'
import { Badge, Dot } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Card, ErrorState, Notice } from '../../components/ui/States'
import { useToast } from '../../components/toast/context'
import { useI18n, type Key } from '../../i18n/context'
import { thisDeviceLabel, type PushState } from '../../lib/push'
import type { Tone } from '../../components/ui/styles'
import type { usePush } from '../../hooks/usePush'
import { pushErrorText } from './pushErrorText'

type Push = ReturnType<typeof usePush>

const BADGE: Partial<Record<PushState, { tone: Tone; label: Key }>> = {
  on: { tone: 'green', label: 'push.state.on' },
  off: { tone: 'gray', label: 'push.state.off' },
  blocked: { tone: 'red', label: 'push.state.blocked' },
  unsupported: { tone: 'gray', label: 'push.state.unsupported' },
  'ios-needs-install': { tone: 'yellow', label: 'push.state.ios' },
  'server-disabled': { tone: 'yellow', label: 'push.state.server' },
}

/** State of push on this browser with the one action that changes it. */
export function ThisDevice({ push }: { push: Push }) {
  const { t, lang } = useI18n()
  const toast = useToast()
  const label = thisDeviceLabel()
  const mobile = /iPhone|iPad|Android/.test(label)
  const badge = BADGE[push.state]

  const enable = () =>
    push.enable.mutate(undefined, {
      onSuccess: () => toast.success(t('push.enabled')),
      onError: (e) => toast.error(pushErrorText(e, lang, t)),
    })
  const disable = () =>
    push.disable.mutate(undefined, {
      onSuccess: () => toast.success(t('push.disabled')),
      onError: (e) => toast.error(pushErrorText(e, lang, t)),
    })

  let body
  switch (push.state) {
    case 'loading':
      body = (
        <div className="space-y-2" aria-hidden="true">
          <div className="h-4 w-3/4 animate-pulse rounded bg-gray-100" />
          <div className="h-9 w-48 animate-pulse rounded-lg bg-gray-100" />
        </div>
      )
      break
    case 'error':
      body = <ErrorState error={push.error} onRetry={() => void push.retry()} />
      break
    case 'on':
      body = (
        <>
          <p className="text-sm text-gray-600">{t('push.on.body')}</p>
          <Button variant="secondary" className="mt-4" onClick={disable} loading={push.disable.isPending}>
            <BellOff className="h-4 w-4" aria-hidden="true" />
            {t('push.disable')}
          </Button>
        </>
      )
      break
    case 'off':
      body = (
        <>
          {push.keyRotated && <Notice tone="warn">{t('push.keyRotated')}</Notice>}
          <p className="text-sm text-gray-600">{t('push.off.body')}</p>
          <Button className="mt-4" onClick={enable} loading={push.enable.isPending}>
            <BellRing className="h-4 w-4" aria-hidden="true" />
            {t('push.enable')}
          </Button>
        </>
      )
      break
    case 'blocked':
      body = (
        <>
          <p className="text-sm font-medium text-gray-900">{t('push.blocked.body')}</p>
          <p className="mt-1 text-sm text-gray-600">{t('push.blocked.how')}</p>
        </>
      )
      break
    case 'ios-needs-install':
      body = (
        <>
          <p className="text-sm font-semibold text-gray-900">{t('push.ios.title')}</p>
          <p className="mt-1 text-sm text-gray-600">{t('push.ios.body')}</p>
          <ol className="mt-3 list-decimal space-y-1 pl-5 text-sm text-gray-700">
            <li>{t('push.ios.step1')}</li>
            <li>{t('push.ios.step2')}</li>
            <li>{t('push.ios.step3')}</li>
          </ol>
        </>
      )
      break
    case 'server-disabled':
      body = <p className="text-sm text-gray-600">{t('push.server.body')}</p>
      break
    case 'unsupported':
      body = <p className="text-sm text-gray-600">{t('push.unsupported.body')}</p>
      break
  }

  return (
    <Card
      title={t('push.device.title')}
      description={label}
      icon={mobile ? Smartphone : Monitor}
      actions={
        badge && (
          <Badge tone={badge.tone}>
            <Dot tone={badge.tone} pulse={push.state === 'on'} />
            {t(badge.label)}
          </Badge>
        )
      }
    >
      {body}
    </Card>
  )
}
