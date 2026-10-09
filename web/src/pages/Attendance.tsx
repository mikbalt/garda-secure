import { useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useSearchParams } from 'react-router-dom'
import { ClipboardCheck } from 'lucide-react'
import { RequireSite } from '../components/RequireSite'
import { PageHeader } from '../components/ui/States'
import { Tabs } from '../components/ui/Tabs'
import { useAuth } from '../hooks/useAuth'
import { useTabParam } from '../hooks/useTabParam'
import { useT } from '../i18n/context'
import { apiGet } from '../api/client'
import { todayIn } from '../lib/time'
import { DailyTab } from './attendance/DailyTab'
import { ReviewTab } from './attendance/ReviewTab'
import type { AttendanceRecord, Site } from '../types'

type TabKey = 'daily' | 'review'

const DATE_RE = /^\d{4}-\d{2}-\d{2}$/

function AttendanceContent({ site }: { site: Site }) {
  const t = useT()
  const { can } = useAuth()
  const canReview = can('reviewAttendance')
  const [tab, setTab] = useTabParam<TabKey>(canReview ? ['daily', 'review'] : ['daily'], 'daily')
  const [params, setParams] = useSearchParams()

  // The selected day lives in ?date= so it survives reloads and can be linked.
  const today = todayIn(site.timezone)
  const raw = params.get('date')
  const date = raw && DATE_RE.test(raw) ? raw : today
  const setDate = useCallback(
    (d: string, openDaily = false) =>
      setParams(
        (prev) => {
          const next = new URLSearchParams(prev)
          if (d === today) next.delete('date')
          else next.set('date', d)
          if (openDaily) next.set('tab', 'daily')
          return next
        },
        { replace: true }
      ),
    [setParams, today]
  )

  const review = useQuery({
    queryKey: ['attendance-review', site.id],
    queryFn: () => apiGet<AttendanceRecord[]>('/attendance/review', { site_id: site.id }),
    enabled: canReview,
  })

  return (
    <>
      <PageHeader icon={ClipboardCheck} title={t('att.title')} subtitle={t('ppl.att.subtitle')} />
      {canReview && (
        <div className="mb-4">
          <Tabs
            label={t('att.title')}
            active={tab}
            onChange={setTab}
            tabs={[
              { key: 'daily', label: t('att.tabDaily') },
              { key: 'review', label: t('ppl.att.tabReview'), badge: review.data?.length },
            ]}
          />
        </div>
      )}
      {tab === 'daily' ? (
        <DailyTab site={site} date={date} today={today} onDateChange={(d) => setDate(d)} />
      ) : (
        <ReviewTab site={site} query={review} onOpenDay={(d) => setDate(d, true)} />
      )}
    </>
  )
}

export default function Attendance() {
  return <RequireSite>{(site) => <AttendanceContent site={site} />}</RequireSite>
}
