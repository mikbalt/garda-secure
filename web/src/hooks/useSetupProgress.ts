import { useQuery } from '@tanstack/react-query'
import { Building2, CalendarClock, BellRing, QrCode, Tablet, UserPlus, type LucideIcon } from 'lucide-react'
import { apiGet } from '../api/client'
import { useSite } from '../context/site'
import { useAuth } from './useAuth'
import type { Key, Vars } from '../i18n/context'
import type { Device, Guard, NotificationRule, PatrolRoute, ShiftTemplate } from '../types'

export type StepState = 'done' | 'partial' | 'todo'

export interface SetupStep {
  key: 'site' | 'guards' | 'schedule' | 'routes' | 'device' | 'notif'
  icon: LucideIcon
  title: Key
  body: Key
  cta: Key
  /** Where the call to action goes (opens the "new" form when possible). */
  to: string
  state: StepState
  status?: { key: Key; vars: Vars }
}

/**
 * Setup checklist for the selected location (admin only). Each step is
 * derived from live data, so it completes itself as the admin works.
 */
export function useSetupProgress() {
  const { can } = useAuth()
  const { sites, siteId, site, isLoading: sitesLoading } = useSite()
  const enabled = can('viewSettings') && !!siteId
  const opts = { enabled, staleTime: 15_000 }

  const guards = useQuery({ queryKey: ['guards', siteId], queryFn: () => apiGet<Guard[]>('/guards', { site_id: siteId }), ...opts })
  const templates = useQuery({ queryKey: ['shift-templates', siteId], queryFn: () => apiGet<ShiftTemplate[]>('/shift-templates', { site_id: siteId }), ...opts })
  const routes = useQuery({ queryKey: ['patrol-routes', siteId], queryFn: () => apiGet<PatrolRoute[]>('/patrol/routes', { site_id: siteId }), ...opts })
  const devices = useQuery({ queryKey: ['devices'], queryFn: () => apiGet<Device[]>('/devices'), ...opts })
  const rules = useQuery({ queryKey: ['rules', siteId], queryFn: () => apiGet<NotificationRule[]>('/notifications/rules', { site_id: siteId }), ...opts })

  const activeGuards = (guards.data ?? []).filter((g) => g.status === 'active')
  const noPin = activeGuards.filter((g) => !g.has_pin).length
  const checkpoints = (routes.data ?? []).reduce((n, r) => n + r.checkpoints.length, 0)
  const siteDevices = (devices.data ?? []).filter((d) => d.site_id === siteId && d.status === 'active').length
  const pendingDevices = (devices.data ?? []).filter((d) => d.status === 'pending').length

  const steps: SetupStep[] = [
    {
      key: 'site', icon: Building2, title: 'setup.step.site.title', body: 'setup.step.site.body', cta: 'setup.step.site.cta',
      to: '/settings?tab=sites&new=1', state: sites.length > 0 ? 'done' : 'todo',
    },
    {
      key: 'guards', icon: UserPlus, title: 'setup.step.guards.title', body: 'setup.step.guards.body',
      cta: activeGuards.length && noPin > 0 ? 'setup.step.guards.ctaPin' : 'setup.step.guards.cta',
      to: activeGuards.length ? '/guards' : '/guards?new=1',
      state: activeGuards.length === 0 ? 'todo' : noPin > 0 ? 'partial' : 'done',
      status: activeGuards.length ? { key: 'setup.step.guards.status', vars: { n: activeGuards.length, m: noPin } } : undefined,
    },
    {
      key: 'schedule', icon: CalendarClock, title: 'setup.step.schedule.title', body: 'setup.step.schedule.body', cta: 'setup.step.schedule.cta',
      to: '/schedules?tab=templates&new=1', state: (templates.data?.length ?? 0) > 0 ? 'done' : 'todo',
      status: templates.data?.length ? { key: 'setup.step.schedule.status', vars: { n: templates.data.length } } : undefined,
    },
    {
      key: 'routes', icon: QrCode, title: 'setup.step.routes.title', body: 'setup.step.routes.body', cta: 'setup.step.routes.cta',
      to: '/patrol?tab=routes&new=1', state: (routes.data?.length ?? 0) > 0 ? 'done' : 'todo',
      status: routes.data?.length ? { key: 'setup.step.routes.status', vars: { n: routes.data.length, m: checkpoints } } : undefined,
    },
    {
      key: 'device', icon: Tablet, title: 'setup.step.device.title', body: 'setup.step.device.body', cta: 'setup.step.device.cta',
      to: '/settings?tab=devices', state: siteDevices > 0 ? 'done' : pendingDevices > 0 ? 'partial' : 'todo',
      status: siteDevices + pendingDevices > 0 ? { key: 'setup.step.device.status', vars: { n: siteDevices, m: pendingDevices } } : undefined,
    },
    {
      key: 'notif', icon: BellRing, title: 'setup.step.notif.title', body: 'setup.step.notif.body', cta: 'setup.step.notif.cta',
      to: '/settings?tab=rules&new=1', state: (rules.data?.length ?? 0) > 0 ? 'done' : 'todo',
      status: rules.data?.length ? { key: 'setup.step.notif.status', vars: { n: rules.data.length } } : undefined,
    },
  ]

  const done = steps.filter((s) => s.state === 'done').length
  const isLoading = sitesLoading || (enabled && [guards, templates, routes, devices, rules].some((q) => q.isLoading))
  return {
    enabled: can('viewSettings'),
    site,
    steps,
    done,
    total: steps.length,
    complete: done === steps.length,
    next: steps.find((s) => s.state !== 'done'),
    isLoading,
  }
}
