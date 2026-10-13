import { useId, useRef, useState, type ReactNode } from 'react'
import { Check, Copy, KeyRound, TriangleAlert } from 'lucide-react'
import { useT } from '../i18n/context'
import { Button } from './ui/Button'

/** Copies text to the clipboard; falls back to selecting the input (non-secure contexts). */
async function copyText(text: string, input: HTMLInputElement | null): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch {
    /* fall through to the selection fallback */
  }
  if (!input) return false
  input.focus()
  input.select()
  try {
    return document.execCommand('copy')
  } catch {
    return false
  }
}

/**
 * A secret the server returns exactly once (temporary password, tablet
 * password): a clear "shown only once" warning, the value in a read-only
 * monospace field and a copy button. The value is never stored.
 */
export function SecretOnce({ label, value, children }: { label: string; value: string; children?: ReactNode }) {
  const t = useT()
  const id = useId()
  const input = useRef<HTMLInputElement>(null)
  const [copied, setCopied] = useState<boolean | null>(null)
  return (
    <div className="space-y-3">
      <div className="flex items-start gap-2.5 rounded-xl border border-amber-200 bg-amber-50 px-3.5 py-2.5 text-sm text-amber-900">
        <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0 text-amber-600" aria-hidden="true" />
        <div>
          <p className="font-semibold">{t('acc.secret.onceTitle')}</p>
          <p className="mt-0.5 text-amber-900">{t('acc.secret.onceBody')}</p>
        </div>
      </div>
      <div>
        <label htmlFor={id} className="mb-1.5 flex items-center gap-1.5 text-sm font-medium text-gray-700">
          <KeyRound className="h-4 w-4 text-gray-500" aria-hidden="true" />
          {label}
        </label>
        <div className="flex gap-2">
          <input
            ref={input}
            id={id}
            readOnly
            value={value}
            spellCheck={false}
            autoComplete="off"
            onFocus={(e) => e.currentTarget.select()}
            className="block w-full min-w-0 rounded-lg border border-gray-300 bg-gray-50 px-3 py-2 font-mono text-sm tracking-wide text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-4 focus:ring-primary-500/15"
            data-autofocus
          />
          <Button variant="secondary" onClick={async () => setCopied(await copyText(value, input.current))} aria-label={`${t('acc.secret.copy')} — ${label}`}>
            {copied ? <Check className="h-4 w-4 text-emerald-600" aria-hidden="true" /> : <Copy className="h-4 w-4" aria-hidden="true" />}
            <span className="hidden sm:inline">{t('acc.secret.copy')}</span>
          </Button>
        </div>
        <p role="status" className="mt-1.5 min-h-[1rem] text-xs text-gray-600">
          {copied === true ? t('acc.secret.copied') : copied === false ? t('acc.secret.copyFailed') : ''}
        </p>
      </div>
      {children}
    </div>
  )
}
