import {
  cloneElement,
  forwardRef,
  isValidElement,
  useId,
  type InputHTMLAttributes,
  type ReactElement,
  type ReactNode,
  type SelectHTMLAttributes,
  type TextareaHTMLAttributes,
} from 'react'
import { useT } from '../../i18n/context'

const control =
  'block w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 shadow-sm placeholder:text-gray-500 focus:border-primary-600 focus:outline-none focus:ring-2 focus:ring-primary-600/30 disabled:bg-gray-100 disabled:text-gray-500 aria-[invalid=true]:border-red-600'

interface FieldProps {
  label: string
  /** A single form control; it receives id / aria-describedby / aria-invalid. */
  children: ReactElement
  hint?: ReactNode
  error?: string
  required?: boolean
  optional?: boolean
  className?: string
}

/** Label + control + hint/error, wired for screen readers. */
export function Field({ label, children, hint, error, required, optional, className = '' }: FieldProps) {
  const t = useT()
  const id = useId()
  const hintId = hint ? `${id}-hint` : undefined
  const errorId = error ? `${id}-error` : undefined
  const describedBy = [hintId, errorId].filter(Boolean).join(' ') || undefined
  const control = isValidElement(children)
    ? cloneElement(children as ReactElement<Record<string, unknown>>, {
        id,
        'aria-describedby': describedBy,
        'aria-invalid': error ? true : undefined,
        required: required || undefined,
      })
    : children
  return (
    <div className={className}>
      <label htmlFor={id} className="mb-1 block text-sm font-medium text-gray-800">
        {label}
        {required && (
          <span className="text-red-700" aria-hidden="true">
            {' '}
            *
          </span>
        )}
        {optional && <span className="font-normal text-gray-600"> ({t('common.optional')})</span>}
      </label>
      {control}
      {hint && (
        <p id={hintId} className="mt-1 text-xs text-gray-600">
          {hint}
        </p>
      )}
      {error && (
        <p id={errorId} className="mt-1 text-xs text-red-700">
          {error}
        </p>
      )}
    </div>
  )
}

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(function Input(
  { className = '', ...rest },
  ref
) {
  return <input ref={ref} className={`${control} ${className}`} {...rest} />
})

export const Select = forwardRef<HTMLSelectElement, SelectHTMLAttributes<HTMLSelectElement>>(function Select(
  { className = '', children, ...rest },
  ref
) {
  return (
    <select ref={ref} className={`${control} pr-8 ${className}`} {...rest}>
      {children}
    </select>
  )
})

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaHTMLAttributes<HTMLTextAreaElement>>(function Textarea(
  { className = '', rows = 3, ...rest },
  ref
) {
  return <textarea ref={ref} rows={rows} className={`${control} ${className}`} {...rest} />
})

interface CheckboxProps extends Omit<InputHTMLAttributes<HTMLInputElement>, 'type'> {
  label: string
}

export function Checkbox({ label, className = '', ...rest }: CheckboxProps) {
  return (
    <label className={`inline-flex cursor-pointer items-center gap-2 text-sm text-gray-800 ${className}`}>
      <input
        type="checkbox"
        className="h-4 w-4 rounded border-gray-400 text-primary-700 focus:ring-2 focus:ring-primary-600"
        {...rest}
      />
      {label}
    </label>
  )
}

/** Inline error block (e.g. a failed mutation inside a modal). */
export function FormError({ message }: { message?: string | null }) {
  if (!message) return null
  return (
    <div role="alert" className="rounded-md border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-800">
      {message}
    </div>
  )
}
