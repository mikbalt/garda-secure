import type { ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'
import { Button } from '../../components/ui/Button'
import { Modal } from '../../components/ui/Modal'
import { useT } from '../../i18n/context'

/** Title + one-line purpose of a settings section, with its primary action. */
export function SectionHeader({ title, description, actions }: { title: string; description: ReactNode; actions?: ReactNode }) {
  return (
    <div className="mb-5 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
      <div className="min-w-0 sm:flex-1">
        <h2 className="text-lg font-semibold tracking-tight text-gray-900">{title}</h2>
        <p className="mt-0.5 max-w-2xl text-sm text-gray-500">{description}</p>
      </div>
      {actions && <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}
    </div>
  )
}

/** Small accessible on/off switch. */
export function Switch({
  checked,
  onChange,
  label,
  disabled,
  size = 'md',
}: {
  checked: boolean
  onChange: (v: boolean) => void
  label: string
  disabled?: boolean
  size?: 'sm' | 'md'
}) {
  const track = size === 'sm' ? 'h-4 w-7' : 'h-5 w-9'
  const knob = size === 'sm' ? 'h-3 w-3' : 'h-4 w-4'
  const shift = size === 'sm' ? 'translate-x-3' : 'translate-x-4'
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      title={label}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={`relative inline-flex shrink-0 items-center rounded-full p-0.5 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-1 disabled:opacity-50 ${track} ${
        checked ? 'bg-emerald-500' : 'bg-gray-300'
      }`}
    >
      <span className={`${knob} rounded-full bg-white shadow-sm transition-transform ${checked ? shift : 'translate-x-0'}`} />
    </button>
  )
}

/** Confirmation dialog for destructive actions (replaces window.confirm). */
export function ConfirmDialog({
  open,
  title,
  message,
  confirmLabel,
  onConfirm,
  onClose,
  loading,
}: {
  open: boolean
  title: string
  message: ReactNode
  confirmLabel: string
  onConfirm: () => void
  onClose: () => void
  loading?: boolean
}) {
  const t = useT()
  return (
    <Modal
      open={open}
      onClose={onClose}
      title={title}
      footer={
        <>
          <Button variant="secondary" onClick={onClose} data-autofocus>
            {t('common.cancel')}
          </Button>
          <Button variant="danger" onClick={onConfirm} loading={loading}>
            {confirmLabel}
          </Button>
        </>
      }
    >
      <div className="text-sm leading-relaxed text-gray-600">{message}</div>
    </Modal>
  )
}

const tint = {
  blue: 'bg-primary-50 text-primary-600 ring-primary-100',
  green: 'bg-emerald-50 text-emerald-600 ring-emerald-100',
  amber: 'bg-amber-50 text-amber-600 ring-amber-100',
  red: 'bg-red-50 text-red-600 ring-red-100',
  orange: 'bg-orange-50 text-orange-600 ring-orange-100',
  violet: 'bg-violet-50 text-violet-600 ring-violet-100',
  gray: 'bg-gray-100 text-gray-500 ring-gray-200',
}
export type TintTone = keyof typeof tint

/** Icon in a tinted rounded square. */
export function IconTile({ icon: Icon, tone = 'blue', size = 'md' }: { icon: LucideIcon; tone?: TintTone; size?: 'sm' | 'md' | 'lg' }) {
  const box = size === 'lg' ? 'h-12 w-12 rounded-xl' : size === 'sm' ? 'h-8 w-8 rounded-lg' : 'h-10 w-10 rounded-xl'
  const ico = size === 'lg' ? 'h-6 w-6' : size === 'sm' ? 'h-4 w-4' : 'h-5 w-5'
  return (
    <span className={`flex shrink-0 items-center justify-center ring-1 ${box} ${tint[tone]}`}>
      <Icon className={ico} aria-hidden="true" />
    </span>
  )
}
