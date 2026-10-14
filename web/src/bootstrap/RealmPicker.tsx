import { ArrowRight, Building2, ClipboardCheck, Loader2, Route, ShieldCheck, Siren } from 'lucide-react'
import { config } from '../config'
import { useI18n, type Key } from '../i18n/context'
import type { Lang } from '../lib/time'

function LangToggle({ dark = false }: { dark?: boolean }) {
  const { lang, setLang, t } = useI18n()
  return (
    <div role="group" aria-label={t('lang.label')} className={`inline-flex rounded-lg p-0.5 ${dark ? 'bg-white/10' : 'bg-gray-100'}`}>
      {(['id', 'en'] as Lang[]).map((l) => (
        <button
          key={l}
          type="button"
          aria-pressed={lang === l}
          aria-label={l === 'id' ? t('lang.id') : t('lang.en')}
          onClick={() => setLang(l)}
          className={`rounded-md px-2.5 py-1 text-xs font-semibold uppercase transition-colors ${
            lang === l ? (dark ? 'bg-white text-gray-900' : 'bg-white text-gray-900 shadow-card') : dark ? 'text-gray-200 hover:text-white' : 'text-gray-600 hover:text-gray-900'
          }`}
        >
          {l}
        </button>
      ))}
    </div>
  )
}

export function BrandMark({ size = 'md' }: { size?: 'md' | 'lg' }) {
  const box = size === 'lg' ? 'h-12 w-12 rounded-2xl' : 'h-9 w-9 rounded-xl'
  return (
    <div className={`flex items-center justify-center bg-gradient-to-br from-primary-400 to-primary-700 shadow-lg shadow-primary-900/40 ring-1 ring-white/20 ${box}`}>
      <ShieldCheck className={size === 'lg' ? 'h-6 w-6 text-white' : 'h-5 w-5 text-white'} aria-hidden="true" />
    </div>
  )
}

const FEATURES: { icon: typeof Route; label: Key }[] = [
  { icon: Route, label: 'org.feature.patrol' },
  { icon: ClipboardCheck, label: 'org.feature.attendance' },
  { icon: Siren, label: 'org.feature.sos' },
]

/** "Choose organisation" screen shown before Keycloak when several realms exist. */
export function RealmPicker({ onChoose }: { onChoose: (realm: string) => void }) {
  const { t } = useI18n()
  return (
    <main className="grid min-h-full bg-white lg:grid-cols-[1fr_minmax(28rem,36rem)]">
      <section className="relative hidden overflow-hidden bg-ink-900 p-12 text-white lg:flex lg:flex-col">
        <div className="pointer-events-none absolute -left-24 -top-24 h-96 w-96 rounded-full bg-primary-500/25 blur-3xl" aria-hidden="true" />
        <div className="pointer-events-none absolute -bottom-40 right-0 h-96 w-96 rounded-full bg-emerald-500/10 blur-3xl" aria-hidden="true" />
        <div className="relative flex items-center gap-3">
          <BrandMark />
          <span className="text-lg font-semibold tracking-tight">{t('app.name')}</span>
        </div>
        <div className="relative mt-auto max-w-lg">
          <h2 className="text-4xl font-semibold leading-tight tracking-tight">
            {t('org.hero')}
          </h2>
          <ul className="mt-8 space-y-4">
            {FEATURES.map(({ icon: Icon, label }) => (
              <li key={label} className="flex items-center gap-3 text-gray-200">
                <span className="flex h-9 w-9 items-center justify-center rounded-xl bg-white/[0.06] text-primary-200 ring-1 ring-white/10">
                  <Icon className="h-[18px] w-[18px]" aria-hidden="true" />
                </span>
                {t(label)}
              </li>
            ))}
          </ul>
        </div>
      </section>

      <section className="flex flex-col p-6 sm:p-10">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2.5 lg:invisible">
            <BrandMark />
            <span className="text-base font-semibold tracking-tight text-gray-900">{t('app.name')}</span>
          </div>
          <LangToggle />
        </div>
        <div className="mx-auto my-auto w-full max-w-sm py-10">
          <h1 className="text-2xl font-semibold tracking-tight text-gray-900">{t('org.choose')}</h1>
          <p className="mb-6 mt-2 text-sm text-gray-600">{t('org.chooseHint')}</p>
          <ul className="space-y-2.5">
            {config.realms.map((r) => (
              <li key={r.id}>
                <button
                  type="button"
                  onClick={() => onChoose(r.id)}
                  className="group flex w-full items-center gap-3.5 rounded-xl border border-gray-200 bg-white px-4 py-3.5 text-left shadow-card transition-all hover:border-primary-300 hover:shadow-raised focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
                >
                  <span className="flex h-10 w-10 items-center justify-center rounded-xl bg-primary-50 text-primary-600 ring-1 ring-primary-100">
                    <Building2 className="h-5 w-5" aria-hidden="true" />
                  </span>
                  <span className="flex-1 font-medium text-gray-900">{r.label}</span>
                  <ArrowRight className="h-4 w-4 text-gray-500 transition-transform group-hover:translate-x-0.5 group-hover:text-primary-500" aria-hidden="true" />
                </button>
              </li>
            ))}
          </ul>
        </div>
      </section>
    </main>
  )
}

/** Centered brand + spinner while Keycloak initialises. */
export function LoadingScreen({ label }: { label: string }) {
  return (
    <main role="status" aria-live="polite" className="flex min-h-full flex-col items-center justify-center gap-5 bg-gray-50">
      <BrandMark size="lg" />
      <span className="inline-flex items-center gap-2 text-sm text-gray-600">
        <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
        {label}
      </span>
    </main>
  )
}

export function FullScreenMessage({
  title,
  body,
  actions,
}: {
  title: string
  body?: string
  actions?: { label: string; onClick: () => void }[]
}) {
  return (
    <main className="flex min-h-full flex-col items-center justify-center bg-gray-50 p-8 text-center">
      <BrandMark size="lg" />
      <h1 className="mt-6 text-xl font-semibold tracking-tight text-gray-900">{title}</h1>
      {body && <p className="mt-2 max-w-md text-sm text-gray-600">{body}</p>}
      {actions && (
        <div className="mt-6 flex flex-wrap justify-center gap-2">
          {actions.map((a, i) => (
            <button
              key={a.label}
              type="button"
              onClick={a.onClick}
              className={`h-9 rounded-lg px-4 text-sm font-medium focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-2 ${
                i === 0 ? 'bg-primary-600 text-white hover:bg-primary-700' : 'bg-white text-gray-800 ring-1 ring-inset ring-gray-300 hover:bg-gray-50'
              }`}
            >
              {a.label}
            </button>
          ))}
        </div>
      )}
    </main>
  )
}
