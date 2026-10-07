import { useMemo, useState, type FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import apiClient from '../../api/client'
import { Button } from '../../components/ui/Button'
import { Field, FormError, Input, Select } from '../../components/ui/Form'
import { Modal } from '../../components/ui/Modal'
import { useToast } from '../../components/toast/context'
import { useSite } from '../../context/site'
import { useI18n, type Key } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import type { Site, SiteConfig, SiteInput } from '../../types'

const DEFAULTS: SiteConfig = {
  clock_window_minutes: 15,
  late_grace_minutes: 5,
  early_out_grace_minutes: 5,
  absent_after_minutes: 30,
  idle_lock_minutes: 5,
  language: 'id',
}

type NumKey = Exclude<keyof SiteConfig, 'language'>
const NUM_FIELDS: { key: NumKey; label: Key; hint?: Key }[] = [
  { key: 'clock_window_minutes', label: 'site.clockWindow', hint: 'site.clockWindowHint' },
  { key: 'late_grace_minutes', label: 'site.lateGrace' },
  { key: 'early_out_grace_minutes', label: 'site.earlyOutGrace' },
  { key: 'absent_after_minutes', label: 'site.absentAfter' },
  { key: 'idle_lock_minutes', label: 'site.idleLock' },
]

function timeZones(): string[] {
  try {
    const fn = (Intl as unknown as { supportedValuesOf?: (k: string) => string[] }).supportedValuesOf
    if (fn) return fn('timeZone')
  } catch {
    // older browsers
  }
  return ['Asia/Jakarta', 'Asia/Makassar', 'Asia/Jayapura', 'UTC']
}

/** Create / edit a site including its operational config_json. */
export function SiteModal({ site, onClose }: { site: Site | null; onClose: () => void }) {
  const { t, lang } = useI18n()
  const qc = useQueryClient()
  const toast = useToast()
  const { setSiteId } = useSite()
  const zones = useMemo(timeZones, [])
  const [name, setName] = useState(site?.name ?? '')
  const [clientId, setClientId] = useState(site?.client_id ?? '')
  const [address, setAddress] = useState(site?.address ?? '')
  const [timezone, setTimezone] = useState(site?.timezone ?? 'Asia/Jakarta')
  const [cfg, setCfg] = useState<Record<NumKey, string>>(() => {
    const c = { ...DEFAULTS, ...(site?.config_json ?? {}) }
    return Object.fromEntries(NUM_FIELDS.map((f) => [f.key, String(c[f.key])])) as Record<NumKey, string>
  })
  const [language, setLanguage] = useState<SiteConfig['language']>(site?.config_json?.language ?? 'id')
  const [formError, setFormError] = useState('')

  const save = useMutation({
    mutationFn: async () => {
      const config_json = { language } as SiteConfig
      for (const f of NUM_FIELDS) config_json[f.key] = Number(cfg[f.key])
      const body: SiteInput = {
        name: name.trim(),
        client_id: clientId.trim() || null,
        address: address.trim() || null,
        timezone: timezone.trim(),
        config_json,
      }
      const res = site ? await apiClient.put<Site>(`/sites/${site.id}`, body) : await apiClient.post<Site>('/sites', body)
      return res.data
    },
    onSuccess: (saved) => {
      toast.success(t('site.created'))
      qc.invalidateQueries({ queryKey: ['sites'] })
      if (!site) setSiteId(saved.id)
      onClose()
    },
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!name.trim() || !timezone.trim()) return setFormError(t('common.required'))
    const bad = NUM_FIELDS.find((f) => {
      const n = Number(cfg[f.key])
      return cfg[f.key] === '' || !Number.isInteger(n) || n < 0 || n > 720
    })
    if (bad) return setFormError(`${t(bad.label)}: 0–720`)
    setFormError('')
    save.mutate()
  }

  return (
    <Modal
      open
      onClose={onClose}
      size="lg"
      title={site ? t('site.edit') : t('site.new')}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="site-form" loading={save.isPending}>
            {t('common.save')}
          </Button>
        </>
      }
    >
      <form id="site-form" onSubmit={submit} className="space-y-4" noValidate>
        <FormError message={formError || (save.error ? errorText(save.error, lang) : null)} />
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label={t('site.name')} required className="sm:col-span-2">
            <Input value={name} onChange={(e) => setName(e.target.value)} maxLength={200} />
          </Field>
          <Field label={t('site.clientId')} optional>
            <Input value={clientId} onChange={(e) => setClientId(e.target.value)} />
          </Field>
          <Field label={t('common.timezone')} required>
            <Input list="tz-list" value={timezone} onChange={(e) => setTimezone(e.target.value)} />
          </Field>
          <datalist id="tz-list">
            {zones.map((z) => (
              <option key={z} value={z} />
            ))}
          </datalist>
          <Field label={t('site.address')} optional className="sm:col-span-2">
            <Input value={address} onChange={(e) => setAddress(e.target.value)} />
          </Field>
        </div>
        <fieldset className="rounded-lg border border-gray-200 p-4">
          <legend className="px-1 text-sm font-semibold text-gray-900">{t('site.operational')}</legend>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            {NUM_FIELDS.map((f) => (
              <Field key={f.key} label={t(f.label)} hint={f.hint ? t(f.hint) : undefined} required>
                <Input
                  type="number"
                  min={0}
                  max={720}
                  step={1}
                  value={cfg[f.key]}
                  onChange={(e) => setCfg((c) => ({ ...c, [f.key]: e.target.value }))}
                />
              </Field>
            ))}
            <Field label={t('site.notifLanguage')} required>
              <Select value={language} onChange={(e) => setLanguage(e.target.value as SiteConfig['language'])}>
                <option value="id">{t('lang.id')}</option>
                <option value="en">{t('lang.en')}</option>
              </Select>
            </Field>
          </div>
        </fieldset>
      </form>
    </Modal>
  )
}
