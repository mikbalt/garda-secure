import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import QRCode from 'qrcode'
import { Printer, RefreshCw } from 'lucide-react'
import { Button } from '../../components/ui/Button'
import { Modal } from '../../components/ui/Modal'
import { Notice } from '../../components/ui/States'
import { useI18n } from '../../i18n/context'
import type { PatrolRoute } from '../../types'

function QrImage({ value, size, alt }: { value: string; size: number; alt: string }) {
  const [src, setSrc] = useState('')
  useEffect(() => {
    let alive = true
    QRCode.toDataURL(value, { margin: 1, width: size, errorCorrectionLevel: 'M' })
      .then((url) => alive && setSrc(url))
      .catch(() => alive && setSrc(''))
    return () => {
      alive = false
    }
  }, [value, size])
  return src ? <img src={src} width={size} height={size} alt={alt} /> : <div style={{ width: size, height: size }} className="bg-gray-100" />
}

function Grid({ route, size }: { route: PatrolRoute; size: number }) {
  const { t } = useI18n()
  return (
    <div className="grid grid-cols-2 gap-4 sm:grid-cols-3">
      {route.checkpoints.map((cp) => (
        <figure key={cp.id} className="flex break-inside-avoid flex-col items-center rounded-xl border border-gray-200 bg-white p-3 text-center">
          <QrImage value={cp.qr_code} size={size} alt={`${t('pat.qrCode')}: ${cp.name}`} />
          <figcaption className="mt-2">
            <div className="text-sm font-semibold text-gray-900">
              {cp.order_index + 1}. {cp.name}
            </div>
            <div className="break-all font-mono text-[11px] text-gray-700">{cp.qr_code}</div>
            <div className="text-xs text-gray-600">{route.name}</div>
          </figcaption>
        </figure>
      ))}
    </div>
  )
}

/** Shows a route's QR codes and prints them as a clean sheet. */
export function QrSheet({
  route,
  onClose,
  onRegenerate,
}: {
  route: PatrolRoute
  onClose: () => void
  /** Admins: regenerate the route's codes (D-037); opens the confirmation. */
  onRegenerate?: () => void
}) {
  const { t } = useI18n()
  const legacy = route.checkpoints.filter((c) => c.qr_signed === false).length

  const print = () => {
    document.body.classList.add('printing-sheet')
    const done = () => {
      document.body.classList.remove('printing-sheet')
      window.removeEventListener('afterprint', done)
    }
    window.addEventListener('afterprint', done)
    // Let images in the print sheet render before the dialog opens.
    window.setTimeout(() => window.print(), 100)
  }

  return (
    <>
      <Modal
        open
        onClose={onClose}
        size="xl"
        title={t('pat.qrSheet', { route: route.name })}
        footer={
          <>
            {onRegenerate && (
              <Button variant="ghost" className="mr-auto text-red-700 hover:bg-red-50 hover:text-red-800" onClick={onRegenerate}>
                <RefreshCw className="h-4 w-4" aria-hidden="true" />
                {t('p2.qr.action')}
              </Button>
            )}
            <Button variant="secondary" onClick={onClose}>
              {t('common.close')}
            </Button>
            <Button onClick={print}>
              <Printer className="h-4 w-4" aria-hidden="true" />
              {t('common.print')}
            </Button>
          </>
        }
      >
        <Notice>{t('ops.pat.qrHint')}</Notice>
        {legacy > 0 && (
          <Notice tone="warn">{t(onRegenerate ? 'p2.qr.legacySheet' : 'p2.qr.legacySheetRo', { n: legacy })}</Notice>
        )}
        <Grid route={route} size={160} />
      </Modal>

      {createPortal(
        <div className="print-sheet" aria-hidden="true">
          <h1 className="mb-4 text-lg font-bold">{t('pat.qrSheet', { route: route.name })}</h1>
          <Grid route={route} size={200} />
        </div>,
        document.body
      )}
    </>
  )
}
