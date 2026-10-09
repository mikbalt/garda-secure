/** Shared style tokens for UI components (kept out of component files for fast refresh). */

export type Tone = 'gray' | 'green' | 'yellow' | 'red' | 'blue' | 'orange' | 'purple'

export type ButtonVariant = 'primary' | 'secondary' | 'danger' | 'ghost' | 'soft'
export type ButtonSize = 'sm' | 'md' | 'lg'

const variants: Record<ButtonVariant, string> = {
  primary:
    'bg-primary-600 text-white shadow-sm shadow-primary-900/10 hover:bg-primary-700 active:bg-primary-800 disabled:bg-primary-300 disabled:shadow-none',
  secondary:
    'bg-white text-gray-800 ring-1 ring-inset ring-gray-300 shadow-sm hover:bg-gray-50 hover:ring-gray-400 active:bg-gray-100 disabled:text-gray-400 disabled:ring-gray-200 disabled:shadow-none',
  danger: 'bg-red-600 text-white shadow-sm hover:bg-red-700 active:bg-red-800 disabled:bg-red-300',
  ghost: 'text-gray-700 hover:bg-gray-100 hover:text-gray-900 active:bg-gray-200 disabled:text-gray-400',
  soft: 'bg-primary-50 text-primary-700 hover:bg-primary-100 active:bg-primary-200 disabled:text-primary-300',
}

const sizes: Record<ButtonSize, string> = {
  sm: 'h-8 px-3 text-xs gap-1.5 rounded-lg',
  md: 'h-9 px-3.5 text-sm gap-2 rounded-lg',
  lg: 'h-11 px-5 text-sm gap-2 rounded-xl',
}

/** Shared class string so links can look like buttons (`<Link className={buttonClass()}>`). */
export function buttonClass(variant: ButtonVariant = 'primary', size: ButtonSize = 'md', extra = ''): string {
  return `inline-flex shrink-0 items-center justify-center whitespace-nowrap font-medium transition-colors duration-150 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-2 disabled:cursor-not-allowed ${variants[variant]} ${sizes[size]} ${extra}`
}

export const statusTone: Record<string, Tone> = {
  present: 'green',
  on_time: 'green',
  sent: 'green',
  succeeded: 'green',
  completed: 'green',
  resolved: 'green',
  active: 'green',
  late: 'yellow',
  in_progress: 'blue',
  running: 'blue',
  queued: 'blue',
  pending: 'gray',
  inactive: 'gray',
  open: 'orange',
  overdue: 'orange',
  absent: 'red',
  missed: 'red',
  failed: 'red',
  revoked: 'red',
  abandoned: 'purple',
}

export const severityTone: Record<string, Tone> = { low: 'gray', medium: 'yellow', high: 'orange', critical: 'red' }

