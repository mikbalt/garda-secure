import type { ReactNode } from 'react'
import { Button } from './Button'
import { Modal } from './Modal'
import { useT } from '../../i18n/context'

/**
 * Confirmation dialog for destructive actions (replaces window.confirm).
 *
 * When `open` is omitted the dialog is always rendered; wrap it in a
 * conditional (`{state && <ConfirmDialog … />}`) to control visibility.
 * When `open` is provided the dialog shows/hides without unmounting.
 */
export function ConfirmDialog({
  open = true,
  title,
  message,
  confirmLabel,
  onConfirm,
  onClose,
  loading,
  error,
}: {
  open?: boolean
  title: string
  message: ReactNode
  confirmLabel: string
  onConfirm: () => void
  onClose: () => void
  loading?: boolean
  error?: ReactNode
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
          <Button variant="danger" loading={loading} onClick={onConfirm}>
            {confirmLabel}
          </Button>
        </>
      }
    >
      <div className="space-y-3 text-sm text-gray-600">
        {error}
        <div>{message}</div>
      </div>
    </Modal>
  )
}
