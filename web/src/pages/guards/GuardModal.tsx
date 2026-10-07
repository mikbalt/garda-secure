import { useState, type FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
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
}

/** Create / edit a guard. PUT is a full replace, so every field is sent. */
export function GuardModal({ guard, defaultSiteId, onClose }: Props) {
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
  const [errors, setErrors] = useState<{ name?: string; site?: string }>({})

  const save = useMutation({
    mutationFn: () => {
      const body: GuardInput = {
        name: name.trim(),
        employee_id: employeeId.trim() || null,
        site_id: siteId,
        role,
        status,
        keycloak_user_id: keycloakUserId.trim() || null,
      }
      return guard ? apiClient.put(`/guards/${guard.id}`, body) : apiClient.post('/guards', body)
    },
    onSuccess: () => {
      toast.success(t('common.saved'))
      qc.invalidateQueries({ queryKey: ['guards'] })
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
      title={guard ? t('grd.edit') : t('grd.new')}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form="guard-form" loading={save.isPending}>
            {t('common.save')}
          </Button>
        </>
      }
    >
      <form id="guard-form" onSubmit={submit} className="space-y-4" noValidate>
        <FormError message={save.error ? errorText(save.error, lang) : null} />
        <Field label={t('common.name')} required error={errors.name}>
          <Input value={name} onChange={(e) => setName(e.target.value)} maxLength={100} autoComplete="off" />
        </Field>
        <Field label={t('grd.employeeId')} optional>
          <Input value={employeeId} onChange={(e) => setEmployeeId(e.target.value)} maxLength={50} autoComplete="off" />
        </Field>
        <Field label={t('common.site')} required error={errors.site}>
          <Select value={siteId} onChange={(e) => setSiteId(e.target.value)}>
            <option value="">—</option>
            {sites.map((s) => (
              <option key={s.id} value={s.id}>
                {s.name}
              </option>
            ))}
          </Select>
        </Field>
        <div className="grid grid-cols-2 gap-4">
          <Field label={t('grd.role')} required>
            <Select value={role} onChange={(e) => setRole(e.target.value as GuardRole)}>
              <option value="guard">{t('role.guard')}</option>
              <option value="supervisor">{t('role.supervisor')}</option>
            </Select>
          </Field>
          <Field label={t('common.status')} required>
            <Select value={status} onChange={(e) => setStatus(e.target.value as ActiveStatus)}>
              <option value="active">{t('status.active')}</option>
              <option value="inactive">{t('status.inactive')}</option>
            </Select>
          </Field>
        </div>
        <Field label={t('grd.keycloakUser')} optional>
          <Input value={keycloakUserId} onChange={(e) => setKeycloakUserId(e.target.value)} autoComplete="off" />
        </Field>
      </form>
    </Modal>
  )
}
