import { useEffect, useRef, useState, type ReactNode } from 'react'

interface PopoverProps {
  /** Renders the trigger; spread `props` onto the button. */
  trigger: (props: { open: boolean; toggle: () => void; 'aria-expanded': boolean; 'aria-haspopup': 'menu' | 'listbox' }) => ReactNode
  children: (close: () => void) => ReactNode
  align?: 'left' | 'right'
  side?: 'bottom' | 'top'
  kind?: 'menu' | 'listbox'
  className?: string
  panelClassName?: string
}

/** Small dropdown panel that closes on outside click and Escape. */
export function Popover({ trigger, children, align = 'left', side = 'bottom', kind = 'menu', className = '', panelClassName = '' }: PopoverProps) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onDoc = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false)
    document.addEventListener('mousedown', onDoc)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDoc)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  const pos = `${align === 'right' ? 'right-0' : 'left-0'} ${side === 'top' ? 'bottom-full mb-2' : 'top-full mt-2'}`

  return (
    <div ref={ref} className={`relative ${className}`}>
      {trigger({ open, toggle: () => setOpen((o) => !o), 'aria-expanded': open, 'aria-haspopup': kind })}
      {open && (
        <div
          role={kind}
          className={`absolute z-50 ${pos} min-w-[14rem] animate-slide-up overflow-hidden rounded-xl border border-gray-200 bg-white py-1.5 shadow-overlay ${panelClassName}`}
        >
          {children(() => setOpen(false))}
        </div>
      )}
    </div>
  )
}
