/** Small accessible on/off switch. */
export function Switch({
  checked,
  onChange,
  label,
  disabled,
  size = 'md',
}: {
  checked: boolean
  onChange: (v: boolean) => void
  label: string
  disabled?: boolean
  size?: 'sm' | 'md'
}) {
  const track = size === 'sm' ? 'h-4 w-7' : 'h-5 w-9'
  const knob = size === 'sm' ? 'h-3 w-3' : 'h-4 w-4'
  const shift = size === 'sm' ? 'translate-x-3' : 'translate-x-4'
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      title={label}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={`relative inline-flex shrink-0 items-center rounded-full p-0.5 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-1 disabled:opacity-50 ${track} ${
        checked ? 'bg-emerald-500' : 'bg-gray-300'
      }`}
    >
      <span className={`${knob} rounded-full bg-white shadow-sm transition-transform ${checked ? shift : 'translate-x-0'}`} />
    </button>
  )
}
