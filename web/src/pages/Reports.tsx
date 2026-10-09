import { Link, useSearchParams } from 'react-router-dom'
import { ArrowLeft, ArrowRight, BarChart3, FileSpreadsheet, Printer, Users } from 'lucide-react'
import { RequireSite } from '../components/RequireSite'
import { PageHeader } from '../components/ui/States'
import { useAuth } from '../hooks/useAuth'
import { useI18n } from '../i18n/context'
import { REPORTS, toneClass, type ReportDef } from './reports/catalog'
import { DailyReport } from './reports/DailyReport'
import { MonthlyReport } from './reports/MonthlyReport'
import { PatrolReport } from './reports/PatrolReport'
import { IncidentReport } from './reports/IncidentReport'
import { KpiReport } from './reports/KpiReport'
import type { Site } from '../types'

function ReportCard({ def }: { def: ReportDef }) {
  const { t } = useI18n()
  const Icon = def.icon
  return (
    <Link
      to={`/reports?tab=${def.key}`}
      className="group flex h-full flex-col rounded-2xl border border-gray-200/80 bg-white p-5 shadow-card transition-all hover:-translate-y-0.5 hover:border-gray-300 hover:shadow-raised focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
    >
      <div className="flex items-start justify-between gap-3">
        <span className={`flex h-11 w-11 items-center justify-center rounded-xl ring-1 ${toneClass[def.tone]}`}>
          <Icon className="h-5 w-5" aria-hidden="true" />
        </span>
        <ArrowRight className="h-4 w-4 text-gray-300 transition-all group-hover:translate-x-0.5 group-hover:text-primary-500" aria-hidden="true" />
      </div>
      <h2 className="mt-4 text-[15px] font-semibold text-gray-900">{t(def.title)}</h2>
      <p className="mt-1 flex-1 text-sm leading-relaxed text-gray-600">{t(def.question)}</p>
      <div className="mt-4 flex flex-wrap items-center justify-between gap-2 border-t border-gray-100 pt-3 text-xs text-gray-500">
        <span className="inline-flex items-center gap-1.5">
          <Users className="h-3.5 w-3.5" aria-hidden="true" />
          {t(def.audience)}
        </span>
        <span className="inline-flex items-center gap-2 text-gray-400">
          <span className="inline-flex items-center gap-1">
            <FileSpreadsheet className="h-3.5 w-3.5" aria-hidden="true" />
            CSV
          </span>
          <span className="inline-flex items-center gap-1">
            <Printer className="h-3.5 w-3.5" aria-hidden="true" />
            PDF
          </span>
        </span>
      </div>
    </Link>
  )
}

function Landing({ reports, site }: { reports: ReportDef[]; site: Site }) {
  const { t } = useI18n()
  return (
    <>
      <PageHeader icon={BarChart3} title={t('rep.title')} subtitle={t('adm.rep.subtitle', { site: site.name })} />
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
        {reports.map((r) => (
          <ReportCard key={r.key} def={r} />
        ))}
        <div className="flex flex-col justify-center rounded-2xl border border-dashed border-gray-300 bg-white/50 p-5 text-sm text-gray-600">
          <p className="font-semibold text-gray-900">{t('adm.rep.tipTitle')}</p>
          <p className="mt-1 leading-relaxed">{t('adm.rep.tipBody')}</p>
        </div>
      </div>
    </>
  )
}

function ReportView({ def, site }: { def: ReportDef; site: Site }) {
  const { t } = useI18n()
  const title = t(def.title)
  return (
    <>
      <div className="no-print">
        <Link
          to="/reports"
          className="mb-3 inline-flex items-center gap-1.5 rounded-md text-sm font-medium text-gray-500 transition-colors hover:text-gray-900 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
        >
          <ArrowLeft className="h-4 w-4" aria-hidden="true" />
          {t('adm.rep.back')}
        </Link>
        <PageHeader icon={def.icon} title={title} subtitle={t(def.question)} />
      </div>
      {def.key === 'daily' && <DailyReport site={site} title={title} />}
      {def.key === 'monthly' && <MonthlyReport site={site} title={title} />}
      {def.key === 'patrol' && <PatrolReport site={site} title={title} />}
      {def.key === 'incidents' && <IncidentReport site={site} title={title} />}
      {def.key === 'kpi' && <KpiReport site={site} title={title} />}
    </>
  )
}

export default function Reports() {
  const { can } = useAuth()
  const [params] = useSearchParams()
  const allowed = REPORTS.filter((r) => can(r.cap))
  const active = allowed.find((r) => r.key === params.get('tab'))
  return (
    <RequireSite>
      {(site) => (active ? <ReportView key={`${active.key}-${site.id}`} def={active} site={site} /> : <Landing reports={allowed} site={site} />)}
    </RequireSite>
  )
}
