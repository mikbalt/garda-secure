import { useCallback, useMemo, useRef, useState, type ReactNode } from 'react'
import { CheckCircle2, AlertCircle, Info, X } from 'lucide-react'
import { ToastContext, type Toast, type ToastKind } from './context'
import { useT } from '../../i18n/context'

const styles: Record<ToastKind, string> = {
  success: 'border-green-600 bg-green-50 text-green-900',
  error: 'border-red-600 bg-red-50 text-red-900',
  info: 'border-primary-600 bg-primary-50 text-primary-900',
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const t = useT()
  const [toasts, setToasts] = useState<Toast[]>([])
  const nextId = useRef(1)

  const dismiss = useCallback((id: number) => setToasts((ts) => ts.filter((x) => x.id !== id)), [])

  const show = useCallback(
    (kind: ToastKind, message: string) => {
      const id = nextId.current++
      setToasts((ts) => [...ts.slice(-4), { id, kind, message }])
      window.setTimeout(() => dismiss(id), kind === 'error' ? 8000 : 4000)
    },
    [dismiss]
  )

  const api = useMemo(() => ({ show }), [show])
  const errors = toasts.filter((x) => x.kind === 'error')
  const others = toasts.filter((x) => x.kind !== 'error')

  const render = (x: Toast) => {
    const Icon = x.kind === 'success' ? CheckCircle2 : x.kind === 'error' ? AlertCircle : Info
    return (
      <div
        key={x.id}
        className={`pointer-events-auto flex items-start gap-2 rounded-lg border-l-4 px-4 py-3 shadow-lg text-sm ${styles[x.kind]}`}
      >
        <Icon className="h-4 w-4 mt-0.5 shrink-0" aria-hidden="true" />
        <p className="flex-1">{x.message}</p>
        <button
          type="button"
          onClick={() => dismiss(x.id)}
          className="rounded p-0.5 hover:bg-black/5 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-600"
          aria-label={t('common.close')}
        >
          <X className="h-4 w-4" aria-hidden="true" />
        </button>
      </div>
    )
  }

  return (
    <ToastContext.Provider value={api}>
      {children}
      <div className="pointer-events-none fixed bottom-4 right-4 z-[60] flex w-full max-w-sm flex-col gap-2 no-print">
        {/* Live regions carry text only; the visible toasts (with their close buttons) are outside them (WEB-6). */}
        <div role="alert" className="sr-only">
          {errors.map((x) => (
            <p key={x.id}>{x.message}</p>
          ))}
        </div>
        <div role="status" className="sr-only">
          {others.map((x) => (
            <p key={x.id}>{x.message}</p>
          ))}
        </div>
        <div className="flex flex-col gap-2">{errors.map(render)}</div>
        <div className="flex flex-col gap-2">{others.map(render)}</div>
      </div>
    </ToastContext.Provider>
  )
}
