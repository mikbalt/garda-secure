import { useId, useRef, type KeyboardEvent } from 'react'

export interface TabItem<K extends string> {
  key: K
  label: string
  badge?: number
}

interface TabsProps<K extends string> {
  tabs: TabItem<K>[]
  active: K
  onChange: (key: K) => void
  label: string
}

/** WAI-ARIA tablist; the panel should use `tabPanelProps(id, key)`. */
export function Tabs<K extends string>({ tabs, active, onChange, label }: TabsProps<K>) {
  const id = useId()
  const refs = useRef<(HTMLButtonElement | null)[]>([])

  const onKey = (e: KeyboardEvent, i: number) => {
    let next = -1
    if (e.key === 'ArrowRight') next = (i + 1) % tabs.length
    if (e.key === 'ArrowLeft') next = (i - 1 + tabs.length) % tabs.length
    if (e.key === 'Home') next = 0
    if (e.key === 'End') next = tabs.length - 1
    if (next >= 0) {
      e.preventDefault()
      onChange(tabs[next].key)
      refs.current[next]?.focus()
    }
  }

  return (
    <div role="tablist" aria-label={label} className="flex flex-wrap gap-1 border-b border-gray-200 no-print">
      {tabs.map((tab, i) => {
        const selected = tab.key === active
        return (
          <button
            key={tab.key}
            ref={(el) => {
              refs.current[i] = el
            }}
            id={`${id}-${tab.key}`}
            role="tab"
            type="button"
            aria-selected={selected}
            tabIndex={selected ? 0 : -1}
            onClick={() => onChange(tab.key)}
            onKeyDown={(e) => onKey(e, i)}
            className={`-mb-px border-b-2 px-4 py-2 text-sm font-medium focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-600 ${
              selected ? 'border-primary-700 text-primary-800' : 'border-transparent text-gray-600 hover:text-gray-900'
            }`}
          >
            {tab.label}
            {tab.badge ? (
              <span className="ml-2 rounded-full bg-red-700 px-1.5 py-0.5 text-xs text-white">{tab.badge}</span>
            ) : null}
          </button>
        )
      })}
    </div>
  )
}
