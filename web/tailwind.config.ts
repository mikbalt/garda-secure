import type { Config } from 'tailwindcss'
import colors from 'tailwindcss/colors'

/**
 * SecurePatrol design tokens.
 * - `gray` is remapped to Tailwind's cooler `slate` so every page shares one
 *   neutral scale.
 * - `primary` is the brand blue (actions, focus, selection).
 * - `ink` is the dark navy of the app shell.
 * Status colours use Tailwind's emerald (ok), amber (warning), rose/red
 * (critical) and sky (in progress).
 */
const config: Config = {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      fontFamily: {
        sans: ['"Inter Variable"', 'Inter', 'ui-sans-serif', 'system-ui', 'Segoe UI', 'Roboto', 'sans-serif'],
      },
      colors: {
        gray: colors.slate,
        primary: {
          50: '#eef4ff',
          100: '#dce7fe',
          200: '#c0d4fe',
          300: '#94b6fc',
          400: '#628ef8',
          500: '#3d68f3',
          600: '#2749e8',
          700: '#1f38d4',
          800: '#2030ab',
          900: '#202f87',
          950: '#171e52',
        },
        ink: {
          700: '#1c2638',
          800: '#141c2b',
          900: '#0d1420',
          950: '#080d16',
        },
      },
      boxShadow: {
        card: '0 1px 2px 0 rgb(15 23 42 / 0.04), 0 1px 3px 0 rgb(15 23 42 / 0.06)',
        raised: '0 4px 12px -2px rgb(15 23 42 / 0.08), 0 2px 4px -2px rgb(15 23 42 / 0.05)',
        overlay: '0 20px 48px -12px rgb(15 23 42 / 0.28)',
      },
      borderRadius: {
        xl: '0.875rem',
        '2xl': '1.125rem',
      },
      keyframes: {
        'fade-in': { from: { opacity: '0' }, to: { opacity: '1' } },
        'slide-up': { from: { opacity: '0', transform: 'translateY(6px)' }, to: { opacity: '1', transform: 'none' } },
        'slide-in-right': { from: { transform: 'translateX(24px)', opacity: '0' }, to: { transform: 'none', opacity: '1' } },
        ping2: { '75%, 100%': { transform: 'scale(2.2)', opacity: '0' } },
      },
      animation: {
        'fade-in': 'fade-in 150ms ease-out',
        'slide-up': 'slide-up 200ms ease-out',
        'slide-in-right': 'slide-in-right 220ms ease-out',
        ping2: 'ping2 1.6s cubic-bezier(0, 0, 0.2, 1) infinite',
      },
    },
  },
  plugins: [],
}

export default config
