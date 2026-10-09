import { useState, type FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ChevronRight } from 'lucide-react'
import apiClient from '../../api/client'
import { Button } from '../../components/ui/Button'
import { Field, FormError, Input, Select } from '../../components/ui/Form'
import { Modal } from '../../components/ui/Modal'
import { useToast } from '../../components/toast/context'
import { useSite } from '../../context/site'
import { useI18n } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import type { ActiveStatus, Guard, GuardInput, GuardRole } from '../../types'

interface Props {
  guard: Guard | null
  defaultSiteId: string
  onClose: () => void
  /** Called after a successful save with the saved guard (`created` for a new one). */
  onSaved?: (guard: Guard, created: boolean) => void
}

/** Create / edit a guard. PUT is a full replace, so every field is sent. */
export function GuardModal({ guard, defaultSiteId, onClose, onSaved }: Props) {
  const { t, lang } = useI18n()
  const { sites } = useSite()
  const qc = useQueryClient()
  const toast = useToast()
  const [name, setName] = useState(guard?.name ?? '')
  const [employeeId, setEmployeeId] = useState(guard?.employee_id ?? '')
  const [siteId, setSiteId] = useState(guard?.site_id ?? defaultSiteId)
  const [role, setRole] = useState<GuardRole>(guard?.role ?? 'guard')
  const [status, setStatus] = useState<ActiveStatus>(guard?.status ?? 'active')
  const [keycloakUserId, setKeycloakUserId] = useState(guard?.keycloak_user_id ?? '')
  const [showAdvanced, setShowAdvanced] = useState(!!guard?.keycloak_user_id)
  const [errors, setErrors] = useState<{ name?: string; site?: string }>({})

  const save = useMutation({
    mutationFn: async () => {
      const body: GuardInput = {
        name: name.trim(),
        employee_id: employeeId.trim() || null,
        site_id: siteId,
        role,
        status,
        keycloak_user_id: keycloakUserId.trim() || null,
      }
      const res = guard ? await apiClient.put<Guard>(`/guards/${guard.id}`, body) : await apiClient.post<Guard>('/guards', body)
      return res.data
    },
    onSuccess: (saved) => {
      toast.success(guard ? t('common.saved') : t('ppl.grd.added', { name: name.trim() }))
      qc.invalidateQueries({ queryKey: ['guards'] })
      onSaved?.(saved && typeof saved === 'object' ? saved : ({ ...guard, name: name.trim() } as Guard), !guard)
      onClose()
    },
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const errs = { name: name.trim() ? undefined : t('common.required'), site: siteId ? undefined : t('common.required') }
    setErrors(errs)
    if (errs.name || errs.site) return
    save.mutate()
  }

  return (
    <Modal
      open
      onClose={onClose}
      title={guard ? t('grd.edit') : t('ppl.grd.add')}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="guard-form" loading={save.isPending}>
            {guard ? t('common.save') : t('ppl.grd.addSubmit')}
          </Button>
        </>
      }
    >
      <form id="guard-form" onSubmit={submit} className="space-y-4" noValidate>
        {!guard && <p className="text-sm text-gray-600">{t('ppl.grd.formIntro')}</p>}
        <FormError message={save.error ? errorText(save.error, lang) : null} />
        <Field label={t('ppl.grd.fullName')} required error={errors.name}>
          <Input value={name} onChange={(e) => setName(e.target.value)} maxLength={100} autoComplete="off" placeholder={t('ppl.grd.namePlaceholder')} />
        </Field>
        <Field label={t('grd.employeeId')} optional hint={t('ppl.grd.empHint')}>
          <Input value={employeeId} onChange={(e) => setEmployeeId(e.target.value)} maxLength={50} autoComplete="off" />
        </Field>
        <Field label={t('common.site')} required error={errors.site} hint={t('ppl.grd.siteHint')}>
          <Select value={siteId} onChange={(e) => setSiteId(e.target.value)}>
            <option value="">—</option>
            {sites.map((s) => (
              <option key={s.id} value={s.id}>
                {s.name}
              </option>
            ))}
          </Select>
        </Field>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label={t('grd.role')} required>
            <Select value={role} onChange={(e) => setRole(e.target.value as GuardRole)}>
              <option value="guard">{t('role.guard')}</option>
              <option value="supervisor">{t('role.supervisor')}</option>
            </Select>
          </Field>
          <Field label={t('common.status')} required hint={status === 'inactive' ? t('ppl.grd.statusHint') : undefined}>
            <Select value={status} onChange={(e) => setStatus(e.target.value as ActiveStatus)}>
              <option value="active">{t('status.active')}</option>
              <option value="inactive">{t('status.inactive')}</option>
            </Select>
          </Field>
        </div>
        <div>
          <button
            type="button"
            aria-expanded={showAdvanced}
            onClick={() => setShowAdvanced((v) => !v)}
            className="inline-flex items-center gap-1 rounded text-sm font-medium text-gray-600 hover:text-gray-900 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
          >
            <ChevronRight className={`h-4 w-4 transition-transform ${showAdvanced ? 'rotate-90' : ''}`} aria-hidden="true" />
            {t('ppl.grd.advanced')}
          </button>
          {showAdvanced && (
            <Field label={t('grd.keycloakUser')} optional hint={t('ppl.grd.keycloakHint')} className="mt-3">
              <Input value={keycloakUserId} onChange={(e) => setKeycloakUserId(e.target.value)} autoComplete="off" />
            </Field>
          )}
        </div>
      </form>
    </Modal>
  )
}
