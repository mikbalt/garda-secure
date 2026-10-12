import { Flag } from 'lucide-react'
import { useI18n } from '../i18n/context'
import { flagText } from '../lib/flags'
import { Popover } from './ui/Popover'

/**
 * Plausibility flags of a record (docs/api.md "Plausibility flags"): one
 * amber badge per flag; activating it explains the flag (disclosure
 * popover). Unknown flags are shown generically.
 */
export function FlagBadges({ flags, compact = false }: { flags?: string[] | null; compact?: boolean }) {
  const { t, lang } = useI18n()
  if (!flags || flags.length === 0) return null
  return (
    <span className="inline-flex flex-wrap items-center gap-1">
      {flags.map((f) => {
        const { label, desc } = flagText(f, lang)
        return (
          <Popover
            key={f}
            label={label}
            panelClassName="w-72 max-w-[calc(100vw-2rem)] px-3.5 py-3"
            trigger={({ toggle, open: _open, ...aria }) => (
              <button
                type="button"
                onClick={toggle}
                {...aria}
                aria-label={t('flag.explain', { flag: label })}
                title={desc}
                className="inline-flex items-center gap-1 whitespace-nowrap rounded-full bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-900 ring-1 ring-inset ring-amber-300 hover:bg-amber-100 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
              >
                <Flag className="h-3 w-3" aria-hidden="true" />
                {compact ? <span className="sr-only">{label}</span> : label}
              </button>
            )}
          >
            {() => (
              <>
                <p className="text-sm font-semibold text-gray-900">{label}</p>
                <p className="mt-1 text-sm text-gray-700">{desc}</p>
              </>
            )}
          </Popover>
        )
      })}
    </span>
  )
}
