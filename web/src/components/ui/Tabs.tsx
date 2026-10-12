import { useRef, type KeyboardEvent, type ReactNode } from 'react'

export interface TabItem<K extends string> {
  key: K
  label: string
  badge?: number
  /** `alert` (red, default) for things needing action; `neutral` for plain counts. */
  badgeTone?: 'alert' | 'neutral'
}

interface TabsProps<K extends string> {
  tabs: TabItem<K>[]
  active: K
  onChange: (key: K) => void
  label: string
  /** Stable id prefix shared with the `TabPanel` (e.g. "att"). */
  idBase: string
}

const tabId = (base: string, key: string) => `${base}-tab-${key}`
const panelId = (base: string, key: string) => `${base}-panel-${key}`

/**
 * WAI-ARIA tablist (automatic activation, arrow/Home/End keys). Render the
 * active tab's content in `<TabPanel idBase={…} active={…}>`.
 */
export function Tabs<K extends string>({ tabs, active, onChange, label, idBase }: TabsProps<K>) {
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
    <div role="tablist" aria-label={label} className="no-print inline-flex max-w-full flex-wrap gap-1 rounded-xl bg-gray-200/60 p-1">
      {tabs.map((tab, i) => {
        const selected = tab.key === active
        return (
          <button
            key={tab.key}
            ref={(el) => {
              refs.current[i] = el
            }}
            id={tabId(idBase, tab.key)}
            role="tab"
            type="button"
            aria-selected={selected}
            // Only the rendered panel exists in the DOM.
            aria-controls={selected ? panelId(idBase, tab.key) : undefined}
            tabIndex={selected ? 0 : -1}
            onClick={() => onChange(tab.key)}
            onKeyDown={(e) => onKey(e, i)}
            className={`inline-flex items-center rounded-lg px-3.5 py-1.5 text-sm font-medium transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 ${
              selected ? 'bg-white text-gray-900 shadow-card' : 'text-gray-700 hover:bg-white/60 hover:text-gray-900'
            }`}
          >
            {tab.label}
            {tab.badge ? (
              <span
                className={`ml-2 min-w-[1.25rem] rounded-full px-1.5 py-px text-center text-[11px] font-semibold ${
                  tab.badgeTone === 'neutral' ? 'bg-gray-200 text-gray-700' : 'bg-red-600 text-white'
                }`}
              >
                {tab.badge}
              </span>
            ) : null}
          </button>
        )
      })}
    </div>
  )
}

/** Panel of the active tab, labelled by its tab. */
export function TabPanel({ idBase, active, children, className = '' }: { idBase: string; active: string; children: ReactNode; className?: string }) {
  return (
    <div role="tabpanel" id={panelId(idBase, active)} aria-labelledby={tabId(idBase, active)} tabIndex={0} className={`focus:outline-none ${className}`}>
      {children}
    </div>
  )
}

/** A `TabPanel` when the tab list is shown, otherwise a plain wrapper. */
export function MaybeTabPanel({ tabbed, className = '', ...rest }: { tabbed: boolean; idBase: string; active: string; children: ReactNode; className?: string }) {
  if (tabbed) return <TabPanel {...rest} className={className} />
  return <div className={className}>{rest.children}</div>
}
