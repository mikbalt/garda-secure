import type { ReactNode } from 'react'
import { AlertCircle, Inbox, Info, TriangleAlert, type LucideIcon } from 'lucide-react'
import { useI18n } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import { Button } from './Button'

interface EmptyStateProps {
  message?: string
  /** Optional bold title above the message. */
  title?: string
  icon?: LucideIcon
  children?: ReactNode
  /** Render without the dashed frame (e.g. inside a Card). */
  bare?: boolean
}

export function EmptyState({ message, title, icon: Icon = Inbox, children, bare }: EmptyStateProps) {
  const { t } = useI18n()
  return (
    <div
      className={`flex flex-col items-center justify-center px-6 py-12 text-center ${
        bare ? '' : 'rounded-2xl border border-dashed border-gray-300 bg-white/60'
      }`}
    >
      <div className="mb-3 flex h-12 w-12 items-center justify-center rounded-full bg-gray-100 ring-8 ring-gray-50">
        <Icon className="h-5 w-5 text-gray-500" aria-hidden="true" />
      </div>
      {title && <p className="text-sm font-semibold text-gray-900">{title}</p>}
      <p className={`max-w-sm text-sm text-gray-600 ${title ? 'mt-1' : ''}`}>{message ?? t('common.noData')}</p>
      {children && <div className="mt-4 flex flex-wrap justify-center gap-2">{children}</div>}
    </div>
  )
}

export function ErrorState({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const { t, lang } = useI18n()
  return (
    <div className="flex flex-col items-center rounded-2xl border border-red-200 bg-red-50/70 px-6 py-10 text-center">
      <div className="mb-3 flex h-11 w-11 items-center justify-center rounded-full bg-red-100">
        <AlertCircle className="h-5 w-5 text-red-700" aria-hidden="true" />
      </div>
      {/* The live region holds text only; the retry button sits outside it. */}
      <div role="alert">
        <p className="text-sm font-semibold text-red-900">{t('common.loadError')}</p>
        <p className="mt-1 max-w-md text-sm text-red-800">{errorText(error, lang)}</p>
      </div>
      {onRetry && (
        <Button variant="secondary" size="sm" className="mt-4" onClick={onRetry}>
          {t('app.retry')}
        </Button>
      )}
    </div>
  )
}

interface CardProps {
  title?: string
  description?: ReactNode
  icon?: LucideIcon
  actions?: ReactNode
  children: ReactNode
  className?: string
  /** Remove body padding (for edge-to-edge lists and tables). */
  flush?: boolean
}

export function Card({ title, description, icon: Icon, actions, children, className = '', flush }: CardProps) {
  const hasHeader = title || actions
  return (
    <section className={`rounded-2xl border border-gray-200/80 bg-white shadow-card ${className}`}>
      {hasHeader && (
        <div className={`flex flex-wrap items-start justify-between gap-3 ${flush ? 'border-b border-gray-100 px-5 py-4' : 'px-5 pt-5'}`}>
          <div className="flex min-w-0 items-start gap-3">
            {Icon && (
              <div className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-gray-100 text-gray-600">
                <Icon className="h-4 w-4" aria-hidden="true" />
              </div>
            )}
            <div className="min-w-0">
              {title && <h3 className="text-[15px] font-semibold leading-6 text-gray-900">{title}</h3>}
              {description && <p className="text-sm text-gray-500">{description}</p>}
            </div>
          </div>
          {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
        </div>
      )}
      <div className={flush ? '' : hasHeader ? 'px-5 pb-5 pt-4' : 'p-5'}>{children}</div>
    </section>
  )
}

interface PageHeaderProps {
  title: string
  actions?: ReactNode
  subtitle?: ReactNode
  /** Small label above the title (section name). */
  eyebrow?: string
  icon?: LucideIcon
}

export function PageHeader({ title, actions, subtitle, eyebrow, icon: Icon }: PageHeaderProps) {
  return (
    <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
      <div className="flex min-w-0 flex-1 items-center gap-3.5">
        {Icon && (
          <div className="hidden h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-white text-primary-600 shadow-card ring-1 ring-gray-200/80 sm:flex">
            <Icon className="h-5 w-5" aria-hidden="true" />
          </div>
        )}
        <div className="min-w-0">
          {eyebrow && <p className="text-xs font-semibold uppercase tracking-wider text-primary-600">{eyebrow}</p>}
          <h1 className="text-2xl font-semibold tracking-tight text-gray-900">{title}</h1>
          {subtitle && <p className="mt-1 max-w-3xl text-sm text-gray-500">{subtitle}</p>}
        </div>
      </div>
      {actions && <div className="no-print flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}
    </div>
  )
}

/** Filter bar wrapper. */
export function Toolbar({ children }: { children: ReactNode }) {
  return (
    <div className="no-print mb-4 flex flex-wrap items-end gap-3 rounded-2xl border border-gray-200/80 bg-white px-4 py-3 shadow-card">
      {children}
    </div>
  )
}

export function Notice({ children, tone = 'info' }: { children: ReactNode; tone?: 'info' | 'warn' }) {
  const warn = tone === 'warn'
  const Icon = warn ? TriangleAlert : Info
  return (
    <div
      className={`mb-4 flex items-start gap-2.5 rounded-xl border px-3.5 py-2.5 text-sm ${
        warn ? 'border-amber-200 bg-amber-50 text-amber-900' : 'border-primary-100 bg-primary-50/70 text-primary-900'
      }`}
    >
      <Icon className={`mt-0.5 h-4 w-4 shrink-0 ${warn ? 'text-amber-600' : 'text-primary-600'}`} aria-hidden="true" />
      <div className="min-w-0">{children}</div>
    </div>
  )
}
