import { Link } from 'react-router-dom'
import { ArrowRight, Check, CircleDashed, ListChecks, PartyPopper } from 'lucide-react'
import { PageHeader } from '../components/ui/States'
import { buttonClass } from '../components/ui/styles'
import { useSetupProgress, type SetupStep } from '../hooks/useSetupProgress'
import { useI18n } from '../i18n/context'

function ProgressRing({ done, total }: { done: number; total: number }) {
  const r = 26
  const c = 2 * Math.PI * r
  const pct = total ? done / total : 0
  return (
    <div className="relative h-16 w-16 shrink-0">
      <svg viewBox="0 0 64 64" className="h-16 w-16 -rotate-90" aria-hidden="true">
        <circle cx="32" cy="32" r={r} fill="none" strokeWidth="6" className="stroke-gray-100" />
        <circle
          cx="32"
          cy="32"
          r={r}
          fill="none"
          strokeWidth="6"
          strokeLinecap="round"
          strokeDasharray={c}
          strokeDashoffset={c * (1 - pct)}
          className="stroke-primary-500 transition-[stroke-dashoffset] duration-700"
        />
      </svg>
      <span className="absolute inset-0 flex items-center justify-center text-sm font-semibold tabular-nums text-gray-900">
        {done}/{total}
      </span>
    </div>
  )
}

function StepRow({ step, index, isNext }: { step: SetupStep; index: number; isNext: boolean }) {
  const { t } = useI18n()
  const Icon = step.icon
  const done = step.state === 'done'
  return (
    <li
      className={`relative flex gap-4 rounded-2xl border bg-white p-5 transition-shadow ${
        isNext ? 'border-primary-200 shadow-raised ring-4 ring-primary-50' : 'border-gray-200/80 shadow-card'
      }`}
    >
      <div className="flex flex-col items-center">
        <div
          className={`flex h-10 w-10 items-center justify-center rounded-xl ${
            done ? 'bg-emerald-50 text-emerald-600 ring-1 ring-emerald-100' : isNext ? 'bg-primary-600 text-white shadow-lg shadow-primary-600/30' : 'bg-gray-100 text-gray-500'
          }`}
        >
          {done ? <Check className="h-5 w-5" aria-hidden="true" /> : <Icon className="h-5 w-5" aria-hidden="true" />}
        </div>
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-xs font-semibold tabular-nums text-gray-500">{String(index + 1).padStart(2, '0')}</span>
          <h3 className={`text-[15px] font-semibold ${done ? 'text-gray-500' : 'text-gray-900'}`}>{t(step.title)}</h3>
          {done ? (
            <span className="rounded-full bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700 ring-1 ring-inset ring-emerald-200">{t('setup.done')}</span>
          ) : step.state === 'partial' ? (
            <span className="rounded-full bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-800 ring-1 ring-inset ring-amber-200">{t('setup.inProgress')}</span>
          ) : isNext ? (
            <span className="rounded-full bg-primary-50 px-2 py-0.5 text-xs font-medium text-primary-700 ring-1 ring-inset ring-primary-200">{t('setup.next')}</span>
          ) : null}
        </div>
        <p className="mt-1 max-w-2xl text-sm leading-relaxed text-gray-600">{t(step.body)}</p>
        {step.status && <p className="mt-2 text-xs font-medium text-gray-500">{t(step.status.key, step.status.vars)}</p>}
      </div>
      <div className="flex shrink-0 items-start">
        <Link to={step.to} className={buttonClass(isNext ? 'primary' : done ? 'ghost' : 'secondary', 'md')}>
          {done ? t('setup.open') : t(step.cta)}
          {!done && <ArrowRight className="h-4 w-4" aria-hidden="true" />}
        </Link>
      </div>
    </li>
  )
}

export default function Setup() {
  const { t } = useI18n()
  const s = useSetupProgress()
  const siteName = s.site?.name ?? '—'

  return (
    <div className="mx-auto max-w-4xl">
      <PageHeader icon={ListChecks} title={t('setup.title')} subtitle={t('setup.subtitle', { site: siteName })} />

      <div className="mb-6 flex items-center gap-5 rounded-2xl border border-gray-200/80 bg-white p-5 shadow-card">
        <ProgressRing done={s.done} total={s.total} />
        <div className="min-w-0 flex-1">
          {s.complete ? (
            <>
              <p className="flex items-center gap-2 text-base font-semibold text-gray-900">
                <PartyPopper className="h-5 w-5 text-emerald-600" aria-hidden="true" />
                {t('setup.allDone')}
              </p>
              <p className="mt-0.5 text-sm text-gray-500">{t('setup.allDoneHint')}</p>
            </>
          ) : (
            <>
              <p className="text-base font-semibold text-gray-900">{t('setup.progress', { done: s.done, total: s.total })}</p>
              {s.next && (
                <p className="mt-0.5 flex items-center gap-1.5 text-sm text-gray-500">
                  <CircleDashed className="h-4 w-4 text-primary-500" aria-hidden="true" />
                  {t('setup.cardBody', { step: t(s.next.title) })}
                </p>
              )}
            </>
          )}
          <div className="mt-3 h-2 overflow-hidden rounded-full bg-gray-100">
            <div
              className="h-full rounded-full bg-gradient-to-r from-primary-500 to-emerald-500 transition-[width] duration-700"
              style={{ width: `${Math.round((s.done / s.total) * 100)}%` }}
            />
          </div>
        </div>
      </div>

      <ol className="space-y-3">
        {s.steps.map((step, i) => (
          <StepRow key={step.key} step={step} index={i} isNext={s.next?.key === step.key} />
        ))}
      </ol>
    </div>
  )
}
