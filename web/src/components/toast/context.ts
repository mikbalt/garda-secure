import { createContext, useCallback, useContext } from 'react'
import { useI18n } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import { blobErrorBody } from '../../api/client'

export type ToastKind = 'success' | 'error' | 'info'

export interface Toast {
  id: number
  kind: ToastKind
  message: string
}

export interface ToastApi {
  show: (kind: ToastKind, message: string) => void
}

export const ToastContext = createContext<ToastApi>({ show: () => undefined })

export function useToast() {
  const { show } = useContext(ToastContext)
  return {
    success: (m: string) => show('success', m),
    error: (m: string) => show('error', m),
    info: (m: string) => show('info', m),
  }
}

/** Returns a handler that shows an API error as a toast. */
export function useErrorToast() {
  const { show } = useContext(ToastContext)
  const { lang } = useI18n()
  return useCallback((err: unknown) => show('error', errorText(err, lang)), [show, lang])
}

/** Like useErrorToast, but understands error bodies of blob downloads. */
export function useDownloadErrorToast() {
  const { show } = useContext(ToastContext)
  const { lang } = useI18n()
  return useCallback(
    async (err: unknown) => show('error', errorText(err, lang, { body: await blobErrorBody(err) })),
    [show, lang]
  )
}
