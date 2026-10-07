import { useQuery } from '@tanstack/react-query'
import { RequireSite } from '../components/RequireSite'
import { PageHeader } from '../components/ui/States'
import { Tabs } from '../components/ui/Tabs'
import { useAuth } from '../hooks/useAuth'
import { useTabParam } from '../hooks/useTabParam'
import { useT } from '../i18n/context'
import { apiGet } from '../api/client'
import { DailyTab } from './attendance/DailyTab'
import { ReviewTab } from './attendance/ReviewTab'
import type { AttendanceRecord, Site } from '../types'

type TabKey = 'daily' | 'review'

function AttendanceContent({ site }: { site: Site }) {
  const t = useT()
  const { can } = useAuth()
  const canReview = can('reviewAttendance')
  const [tab, setTab] = useTabParam<TabKey>(canReview ? ['daily', 'review'] : ['daily'], 'daily')

  const review = useQuery({
    queryKey: ['attendance-review', site.id],
    queryFn: () => apiGet<AttendanceRecord[]>('/attendance/review', { site_id: site.id }),
    enabled: canReview,
  })

  return (
    <>
      {canReview && (
        <Tabs
          label={t('att.title')}
          active={tab}
          onChange={setTab}
          tabs={[
            { key: 'daily', label: t('att.tabDaily') },
            { key: 'review', label: t('att.tabReview'), badge: review.data?.length },
          ]}
        />
      )}
      <div className="pt-4">
        {tab === 'daily' ? <DailyTab site={site} /> : <ReviewTab site={site} query={review} />}
      </div>
    </>
  )
}

export default function Attendance() {
  const t = useT()
  return (
    <div>
      <PageHeader title={t('att.title')} />
      <RequireSite>{(site) => <AttendanceContent site={site} />}</RequireSite>
    </div>
  )
}
