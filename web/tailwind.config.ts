import type { Config } from 'tailwindcss'

/**
 * GARDA design tokens (Buku Jaga, TAQ-42): warm paper, ink actions, flat cards.
 * Colours read CSS variables from src/index.css (light + `.dark`), so a theme
 * switch needs no class changes. `gray-*` is the warm neutral scale, `primary-*`
 * the ink action scale (600 = button), `ink-*` the always-dark app shell, and
 * ok/warn/overdue/danger/sos are the muted status pairs (text + `-bg` tint).
 */
const rgb = (name: string) => `rgb(var(--${name}) / <alpha-value>)`

const config: Config = {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      fontFamily: {
        sans: ['"Inter Variable"', 'Inter', 'ui-sans-serif', 'system-ui', 'Segoe UI', 'Roboto', 'sans-serif'],
      },
      colors: {
        gray: {
          50: rgb('gray-50'),
          100: rgb('gray-100'),
          200: rgb('gray-200'),
          300: rgb('gray-300'),
          400: rgb('gray-400'),
          500: rgb('gray-500'),
          600: rgb('gray-600'),
          700: rgb('gray-700'),
          800: rgb('gray-800'),
          900: rgb('gray-900'),
          950: rgb('gray-950'),
        },
        primary: {
          50: rgb('primary-50'),
          100: rgb('primary-100'),
          200: rgb('primary-200'),
          300: rgb('primary-300'),
          400: rgb('primary-400'),
          500: rgb('primary-500'),
          600: rgb('primary-600'),
          700: rgb('primary-700'),
          800: rgb('primary-800'),
          900: rgb('primary-900'),
          950: rgb('primary-950'),
        },
        // App shell (always dark, both themes).
        ink: {
          700: '#223356',
          800: '#1b2a4a',
          900: '#131d35',
          950: '#0d1424',
        },
        paper: rgb('paper'),
        surface: rgb('surface'),
        'action-fg': rgb('action-fg'),
        ok: { DEFAULT: rgb('ok'), bg: rgb('ok-bg') },
        warn: { DEFAULT: rgb('warn'), bg: rgb('warn-bg') },
        overdue: { DEFAULT: rgb('overdue'), bg: rgb('overdue-bg') },
        danger: { DEFAULT: rgb('danger'), bg: rgb('danger-bg') },
        sos: { DEFAULT: rgb('sos'), fg: rgb('sos-fg') },
      },
      // Flat cards: 1px border, no shadow except overlays.
      boxShadow: {
        card: 'none',
        raised: '0 2px 8px -2px rgb(21 24 30 / 0.12)',
        overlay: '0 20px 48px -12px rgb(21 24 30 / 0.28)',
      },
      // 4pt grid: 6 for inputs/chips, 10 for cards.
      borderRadius: {
        md: '0.375rem',
        lg: '0.375rem',
        xl: '0.625rem',
        '2xl': '0.625rem',
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
