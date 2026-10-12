import { Flag } from 'lucide-react'
import { useI18n } from '../i18n/context'
import { flagText } from '../lib/flags'

/**
 * Non-interactive flag badge for places that are themselves a button (list
 * rows): names the flags; the explanation is in the opened detail view
 * (`FlagBadges`) and in the hover title.
 */
export function FlagIndicator({ flags }: { flags: string[] }) {
  const { lang } = useI18n()
  if (flags.length === 0) return null
  const texts = flags.map((f) => flagText(f, lang))
  return (
    <span
      title={texts.map((x) => `${x.label}: ${x.desc}`).join('\n')}
      className="inline-flex items-center gap-1 whitespace-nowrap rounded-full bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-900 ring-1 ring-inset ring-amber-300"
    >
      <Flag className="h-3 w-3" aria-hidden="true" />
      {texts.map((x) => x.label).join(', ')}
    </span>
  )
}
