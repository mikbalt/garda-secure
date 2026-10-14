/**
 * GARDA app mark. Renders the app icon itself (public/favicon.svg, a copy of
 * branding/icon.svg made by branding/generate.sh), so replacing the icon also
 * changes the mark in the UI.
 */
export function BrandMark({ size = 'md' }: { size?: 'md' | 'lg' }) {
  const box = size === 'lg' ? 'h-12 w-12 rounded-2xl' : 'h-9 w-9 rounded-xl'
  return (
    <img
      src={`${import.meta.env.BASE_URL}favicon.svg`}
      alt=""
      aria-hidden="true"
      draggable={false}
      className={`shrink-0 select-none bg-primary-600 shadow-lg shadow-primary-900/40 ring-1 ring-white/20 ${box}`}
    />
  )
}
