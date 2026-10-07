import { Shield, Building2 } from 'lucide-react'
import { config } from '../config'
import { useI18n } from '../i18n/context'
import type { Lang } from '../lib/time'

function LangToggle() {
  const { lang, setLang, t } = useI18n()
  return (
    <div className="absolute right-4 top-4">
      <label htmlFor="picker-lang" className="sr-only">
        {t('lang.label')}
      </label>
      <select
        id="picker-lang"
        value={lang}
        onChange={(e) => setLang(e.target.value as Lang)}
        className="rounded-md border border-gray-300 bg-white py-1.5 pl-2 pr-7 text-sm"
      >
        <option value="id">{t('lang.id')}</option>
        <option value="en">{t('lang.en')}</option>
      </select>
    </div>
  )
}

/** "Choose organisation" screen shown before Keycloak when several realms exist. */
export function RealmPicker({ onChoose }: { onChoose: (realm: string) => void }) {
  const { t } = useI18n()
  return (
    <main className="relative flex min-h-full items-center justify-center bg-primary-800 p-4">
      <LangToggle />
      <div className="w-full max-w-md rounded-xl bg-white p-8 shadow-xl">
        <div className="mb-6 flex items-center gap-2 text-primary-800">
          <Shield className="h-7 w-7" aria-hidden="true" />
          <span className="text-xl font-bold">{t('app.name')}</span>
        </div>
        <h1 className="text-lg font-semibold text-gray-900">{t('org.choose')}</h1>
        <p className="mb-5 mt-1 text-sm text-gray-700">{t('org.chooseHint')}</p>
        <ul className="space-y-2">
          {config.realms.map((r) => (
            <li key={r.id}>
              <button
                type="button"
                onClick={() => onChoose(r.id)}
                className="flex w-full items-center gap-3 rounded-lg border border-gray-300 px-4 py-3 text-left hover:border-primary-600 hover:bg-primary-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-600"
              >
                <Building2 className="h-5 w-5 text-primary-700" aria-hidden="true" />
                <span className="font-medium text-gray-900">{r.label}</span>
              </button>
            </li>
          ))}
        </ul>
      </div>
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
    <main className="flex min-h-full flex-col items-center justify-center p-8 text-center">
      <h1 className="text-xl font-semibold text-gray-900">{title}</h1>
      {body && <p className="mt-2 max-w-md text-sm text-gray-700">{body}</p>}
      {actions && (
        <div className="mt-4 flex gap-2">
          {actions.map((a) => (
            <button
              key={a.label}
              type="button"
              onClick={a.onClick}
              className="rounded-md bg-primary-700 px-4 py-2 text-sm font-medium text-white hover:bg-primary-800 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-600 focus-visible:ring-offset-2"
            >
              {a.label}
            </button>
          ))}
        </div>
      )}
    </main>
  )
}
