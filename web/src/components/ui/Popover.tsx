import { useEffect, useId, useRef, useState, type ReactNode } from 'react'

export interface PopoverTriggerProps {
  open: boolean
  toggle: () => void
  'aria-expanded': boolean
  'aria-controls': string
}

interface PopoverProps {
  /** Renders the trigger; spread the props (minus `open`/`toggle`) onto the button. */
  trigger: (props: PopoverTriggerProps) => ReactNode
  children: (close: () => void) => ReactNode
  align?: 'left' | 'right'
  side?: 'bottom' | 'top'
  /** Accessible name of the panel. */
  label?: string
  className?: string
  panelClassName?: string
}

/**
 * Small dropdown panel using the WAI-ARIA **disclosure** pattern (WEB-6):
 * the trigger toggles `aria-expanded`, the panel follows it in tab order and
 * holds ordinary buttons/links (no menu/listbox roles, so no arrow-key
 * contract). Escape closes and returns focus to the trigger; clicking or
 * tabbing outside closes it.
 */
export function Popover({ trigger, children, align = 'left', side = 'bottom', label, className = '', panelClassName = '' }: PopoverProps) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  const panelId = useId()

  useEffect(() => {
    if (!open) return
    const root = ref.current
    const onDoc = (e: MouseEvent) => {
      if (root && !root.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return
      setOpen(false)
      root?.querySelector<HTMLElement>('[aria-expanded]')?.focus()
    }
    const onFocusOut = (e: FocusEvent) => {
      const next = e.relatedTarget as Node | null
      if (root && next && !root.contains(next)) setOpen(false)
    }
    document.addEventListener('mousedown', onDoc)
    document.addEventListener('keydown', onKey)
    root?.addEventListener('focusout', onFocusOut)
    return () => {
      document.removeEventListener('mousedown', onDoc)
      document.removeEventListener('keydown', onKey)
      root?.removeEventListener('focusout', onFocusOut)
    }
  }, [open])

  const pos = `${align === 'right' ? 'right-0' : 'left-0'} ${side === 'top' ? 'bottom-full mb-2' : 'top-full mt-2'}`

  return (
    <div ref={ref} className={`relative ${className}`}>
      {trigger({ open, toggle: () => setOpen((o) => !o), 'aria-expanded': open, 'aria-controls': panelId })}
      {open && (
        <div
          id={panelId}
          role={label ? 'group' : undefined}
          aria-label={label}
          className={`absolute z-50 ${pos} min-w-[14rem] animate-slide-up overflow-hidden rounded-xl border border-gray-200 bg-white py-1.5 shadow-overlay ${panelClassName}`}
        >
          {children(() => setOpen(false))}
        </div>
      )}
    </div>
  )
}
