import type { LucideIcon } from 'lucide-react'

const tint = {
  blue: 'bg-primary-50 text-primary-600 ring-primary-100',
  green: 'bg-emerald-50 text-emerald-600 ring-emerald-100',
  amber: 'bg-amber-50 text-amber-600 ring-amber-100',
  red: 'bg-red-50 text-red-600 ring-red-100',
  orange: 'bg-orange-50 text-orange-600 ring-orange-100',
  violet: 'bg-violet-50 text-violet-600 ring-violet-100',
  gray: 'bg-gray-100 text-gray-500 ring-gray-200',
}

export type TintTone = keyof typeof tint

/** Icon in a tinted rounded square. */
export function IconTile({
  icon: Icon,
  tone = 'blue',
  size = 'md',
}: {
  icon: LucideIcon
  tone?: TintTone
  size?: 'sm' | 'md' | 'lg'
}) {
  const box = size === 'lg' ? 'h-12 w-12 rounded-xl' : size === 'sm' ? 'h-8 w-8 rounded-lg' : 'h-10 w-10 rounded-xl'
  const ico = size === 'lg' ? 'h-6 w-6' : size === 'sm' ? 'h-4 w-4' : 'h-5 w-5'
  return (
    <span className={`flex shrink-0 items-center justify-center ring-1 ${box} ${tint[tone]}`}>
      <Icon className={ico} aria-hidden="true" />
    </span>
  )
}
