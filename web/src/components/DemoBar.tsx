import { FlaskConical, RotateCcw } from 'lucide-react'
import { useI18n, type Key } from '../i18n/context'
import { demoRole } from '../keycloak'
import { STORAGE_KEYS, writeStored } from '../lib/storage'

const ROLES = ['admin', 'supervisor', 'hr', 'client-readonly'] as const

async function reset(mode?: 'sample' | 'empty') {
  const { resetDemo } = await import('../demo/mockApi')
  resetDemo(mode)
}

/** Slim bar shown only in demo mode: switch role, reset or empty the data. */
export function DemoBar() {
  const { t } = useI18n()
  const role = demoRole()
  return (
    <div className="no-print flex flex-wrap items-center gap-x-4 gap-y-1.5 bg-gradient-to-r from-violet-600 to-primary-600 px-4 py-1.5 text-xs text-white sm:px-6">
      <span className="inline-flex items-center gap-1.5 font-medium">
        <FlaskConical className="h-3.5 w-3.5" aria-hidden="true" />
        <span className="hidden md:inline">{t('demo.banner')}</span>
        <span className="md:hidden">Demo</span>
      </span>
      <label className="inline-flex items-center gap-1.5">
        <span className="text-white/80">{t('demo.viewAs')}</span>
        <select
          value={role}
          onChange={(e) => {
            writeStored(STORAGE_KEYS.demoRole, e.target.value)
            window.location.assign('/')
          }}
          className="rounded-md border-0 bg-white/15 py-0.5 pl-2 pr-7 text-xs font-medium text-white focus:ring-2 focus:ring-white/60 [&>option]:text-gray-900"
        >
          {ROLES.map((r) => (
            <option key={r} value={r}>
              {t(r === 'supervisor' ? 'role.supervisor' : (`role.${r}` as Key))}
            </option>
          ))}
        </select>
      </label>
      <div className="ml-auto flex items-center gap-1">
        <button type="button" onClick={() => reset('empty')} className="rounded-md px-2 py-0.5 font-medium text-white/90 hover:bg-white/15">
          {t('demo.empty')}
        </button>
        <button type="button" onClick={() => reset('sample')} className="inline-flex items-center gap-1 rounded-md px-2 py-0.5 font-medium text-white/90 hover:bg-white/15">
          <RotateCcw className="h-3 w-3" aria-hidden="true" />
          {t('demo.sample')}
        </button>
      </div>
    </div>
  )
}
