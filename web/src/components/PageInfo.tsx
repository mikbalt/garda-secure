import { Button } from './ui/Button'
import { Notice } from './ui/States'
import { useI18n } from '../i18n/context'
import { formatNumber } from '../lib/time'

/** "Showing 50 of 63" for paged lists (`X-Total-Count`, D-045). */
export function ShowingCount({ shown, total, className = '' }: { shown: number; total: number; className?: string }) {
  const { t, lang } = useI18n()
  if (total <= shown) return null
  return <p className={`text-xs text-gray-600 ${className}`}>{t('p2.page.showing', { n: formatNumber(shown, lang), total: formatNumber(total, lang) })}</p>
}

/**
 * Footer of a paged list: "Load more" while the server has more rows, or a
 * warning when the list is cut off (`X-Truncated`) and cannot page further.
 */
export function PageFooter({
  shown,
  total,
  truncated,
  onMore,
  loadingMore,
}: {
  shown: number
  total: number
  truncated: boolean
  /** Loads the next page; omit when the list cannot page (a warning is shown instead). */
  onMore?: () => void
  loadingMore?: boolean
}) {
  const { t, lang } = useI18n()
  if (!truncated && shown >= total) return null
  if (!onMore) {
    return (
      <div className="mt-4">
        <Notice tone="warn">{t('p2.page.truncated', { n: formatNumber(shown, lang), total: formatNumber(total, lang) })}</Notice>
      </div>
    )
  }
  return (
    <div className="mt-4 flex flex-col items-center gap-2">
      <p className="text-xs text-gray-600">{t('p2.page.showing', { n: formatNumber(shown, lang), total: formatNumber(total, lang) })}</p>
      <Button variant="secondary" onClick={onMore} loading={loadingMore}>
        {t('p2.page.more', { n: formatNumber(Math.max(0, total - shown), lang) })}
      </Button>
    </div>
  )
}
