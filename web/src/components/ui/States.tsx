import type { ReactNode } from 'react'
import { AlertCircle, Inbox } from 'lucide-react'
import { useI18n } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import { Button } from './Button'

export function EmptyState({ message, children }: { message?: string; children?: ReactNode }) {
  const { t } = useI18n()
  return (
    <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-gray-300 bg-white px-4 py-10 text-center">
      <Inbox className="mb-2 h-8 w-8 text-gray-500" aria-hidden="true" />
      <p className="text-sm text-gray-700">{message ?? t('common.noData')}</p>
      {children && <div className="mt-3">{children}</div>}
    </div>
  )
}

export function ErrorState({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const { t, lang } = useI18n()
  return (
    <div role="alert" className="flex flex-col items-center rounded-lg border border-red-200 bg-red-50 px-4 py-8 text-center">
      <AlertCircle className="mb-2 h-6 w-6 text-red-700" aria-hidden="true" />
      <p className="text-sm font-medium text-red-900">{t('common.loadError')}</p>
      <p className="mt-1 text-sm text-red-800">{errorText(error, lang)}</p>
      {onRetry && (
        <Button variant="secondary" size="sm" className="mt-3" onClick={onRetry}>
          {t('app.retry')}
        </Button>
      )}
    </div>
  )
}

export function Card({ title, actions, children, className = '' }: { title?: string; actions?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={`rounded-xl border border-gray-200 bg-white p-5 shadow-sm ${className}`}>
      {(title || actions) && (
        <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
          {title && <h3 className="text-sm font-semibold text-gray-900">{title}</h3>}
          {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
        </div>
      )}
      {children}
    </section>
  )
}

export function PageHeader({ title, actions, subtitle }: { title: string; actions?: ReactNode; subtitle?: ReactNode }) {
  return (
    <div className="mb-5 flex flex-wrap items-end justify-between gap-3">
      <div>
        <h1 className="text-xl font-semibold text-gray-900">{title}</h1>
        {subtitle && <p className="mt-0.5 text-sm text-gray-600">{subtitle}</p>}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2 no-print">{actions}</div>}
    </div>
  )
}

/** Filter bar wrapper. */
export function Toolbar({ children }: { children: ReactNode }) {
  return <div className="mb-4 flex flex-wrap items-end gap-3 no-print">{children}</div>
}

export function Notice({ children, tone = 'info' }: { children: ReactNode; tone?: 'info' | 'warn' }) {
  const cls = tone === 'warn' ? 'border-yellow-300 bg-yellow-50 text-yellow-900' : 'border-primary-200 bg-primary-50 text-primary-900'
  return <p className={`mb-4 rounded-md border px-3 py-2 text-sm ${cls}`}>{children}</p>
}
