import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { useT } from '../../i18n/context'

export interface WeeklyPoint {
  label: string
  present: number
  late: number
  absent: number
}

/**
 * Stacked weekly attendance bars. Loaded lazily (WEB-9) so recharts is not
 * part of the landing page's first paint.
 */
export default function WeeklyChart({ data }: { data: WeeklyPoint[] }) {
  const t = useT()
  return (
    <ResponsiveContainer width="100%" height="100%">
      <BarChart data={data} margin={{ top: 4, right: 0, left: -28, bottom: 0 }} barCategoryGap="28%">
        <CartesianGrid vertical={false} stroke="#eef2f6" />
        <XAxis dataKey="label" tickLine={false} axisLine={false} tick={{ fontSize: 11, fill: '#475569' }} />
        <YAxis allowDecimals={false} tickLine={false} axisLine={false} tick={{ fontSize: 11, fill: '#475569' }} />
        <Tooltip cursor={{ fill: '#f1f5f9' }} contentStyle={{ borderRadius: 12, border: '1px solid #e2e8f0', fontSize: 12 }} />
        <Bar dataKey="present" name={t('status.present')} stackId="a" fill="#10b981" />
        <Bar dataKey="late" name={t('status.late')} stackId="a" fill="#f59e0b" />
        <Bar dataKey="absent" name={t('status.absent')} stackId="a" fill="#ef4444" radius={[4, 4, 0, 0]} />
      </BarChart>
    </ResponsiveContainer>
  )
}
