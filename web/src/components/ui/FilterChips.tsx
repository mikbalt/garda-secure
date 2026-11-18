import { useId } from 'react'
import { Dot, type Tone } from './Badge'

// ── Shared types ─────────────────────────────────────────────────────────────

/** Item for FilterChips / ChipGroup. */
export interface ChipItem<K extends string> {
  key: K
  label: string
  count?: number
  /** Dot colour class (e.g. `bg-emerald-500`). Used by FilterChips. */
  dot?: string
}

/** Option for Segmented / ChipGroup (badge-style). */
export interface ChipOption<V extends string> {
  value: V
  label: string
  count?: number
  tone?: Tone
}

// ── FilterChips ───────────────────────────────────────────────────────────────

/**
 * Toggle chips that filter a list ("Semua 12 · Hadir 8 · …"). Exactly one is
 * active; each button reports its pressed state to screen readers.
 */
export function FilterChips<K extends string>({
  items,
  active,
  onChange,
  label,
}: {
  items: ChipItem<K>[]
  active: K
  onChange: (k: K) => void
  label: string
}) {
  return (
    <div role="group" aria-label={label} className="flex flex-wrap gap-2">
      {items.map((it) => {
        const on = it.key === active
        return (
          <button
            key={it.key}
            type="button"
            aria-pressed={on}
            onClick={() => onChange(it.key)}
            className={`inline-flex h-8 items-center gap-2 rounded-full px-3 text-sm font-medium transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-1 ${
              on
                ? 'bg-ink-900 text-white shadow-sm'
                : 'bg-white text-gray-700 ring-1 ring-inset ring-gray-200 hover:bg-gray-50 hover:ring-gray-300'
            }`}
          >
            {it.dot && <span className={`h-2 w-2 rounded-full ${it.dot}`} aria-hidden="true" />}
            {it.label}
            {it.count !== undefined && (
              <span
                className={`min-w-[1.25rem] rounded-full px-1.5 text-center text-xs tabular-nums ${
                  on ? 'bg-white/15 text-white' : 'bg-gray-100 text-gray-600'
                }`}
              >
                {it.count}
              </span>
            )}
          </button>
        )
      })}
    </div>
  )
}

// ── ChipGroup ─────────────────────────────────────────────────────────────────

/** Toggle chips (single select), e.g. severity filter. */
export function ChipGroup<V extends string>({
  label,
  options,
  value,
  onChange,
}: {
  label: string
  options: ChipOption<V>[]
  value: V
  onChange: (v: V) => void
}) {
  const id = useId()
  return (
    <div>
      <span id={id} className="mb-1.5 block text-sm font-medium text-gray-700">
        {label}
      </span>
      <div role="group" aria-labelledby={id} className="flex flex-wrap gap-1.5">
        {options.map((o) => {
          const on = o.value === value
          return (
            <button
              key={o.value}
              type="button"
              aria-pressed={on}
              onClick={() => onChange(o.value)}
              className={`inline-flex h-8 items-center gap-1.5 rounded-full px-3 text-sm font-medium ring-1 ring-inset transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 ${
                on ? 'bg-gray-900 text-white ring-gray-900' : 'bg-white text-gray-700 ring-gray-300 hover:bg-gray-50'
              }`}
            >
              {o.tone && <Dot tone={o.tone} />}
              {o.label}
            </button>
          )
        })}
      </div>
    </div>
  )
}

// ── Segmented ─────────────────────────────────────────────────────────────────

const groupLabel = 'mb-1.5 block text-sm font-medium text-gray-700'

/** Single-select segmented control (pill group). Buttons use aria-pressed. */
export function Segmented<V extends string>({
  label,
  options,
  value,
  onChange,
  hideLabel = false,
  size = 'md',
  disabled = false,
}: {
  label: string
  options: ChipOption<V>[]
  value: V
  onChange: (v: V) => void
  hideLabel?: boolean
  size?: 'sm' | 'md'
  disabled?: boolean
}) {
  const id = useId()
  return (
    <div>
      <span id={id} className={hideLabel ? 'sr-only' : groupLabel}>
        {label}
      </span>
      <div role="group" aria-labelledby={id} className="inline-flex max-w-full flex-wrap gap-0.5 rounded-lg bg-gray-100 p-0.5 ring-1 ring-inset ring-gray-200/70">
        {options.map((o) => {
          const on = o.value === value
          return (
            <button
              key={o.value}
              type="button"
              aria-pressed={on}
              disabled={disabled}
              onClick={() => onChange(o.value)}
              className={`inline-flex items-center gap-1.5 whitespace-nowrap rounded-md font-medium disabled:cursor-wait disabled:opacity-60 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 ${
                size === 'sm' ? 'h-7 px-2.5 text-xs' : 'h-8 px-3 text-sm'
              } ${on ? 'bg-white text-gray-900 shadow-card' : 'text-gray-600 hover:text-gray-900'}`}
            >
              {o.tone && <Dot tone={o.tone} />}
              {o.label}
              {o.count !== undefined && (
                <span className={`rounded-full px-1.5 text-[11px] tabular-nums ${on ? 'bg-gray-100 text-gray-700' : 'bg-gray-200/70 text-gray-500'}`}>{o.count}</span>
              )}
            </button>
          )
        })}
      </div>
    </div>
  )
}
