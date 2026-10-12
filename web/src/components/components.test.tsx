import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { I18nProvider } from '../i18n/I18nProvider'
import { ErrorBoundary } from './ErrorBoundary'
import { FlagBadges } from './FlagBadges'
import { IdleLogout } from './IdleLogout'
import { TabPanel, Tabs } from './ui/Tabs'
import { Popover } from './ui/Popover'

const withI18n = (ui: React.ReactNode) => render(<I18nProvider>{ui}</I18nProvider>)

describe('FlagBadges', () => {
  it('explains a flag in a disclosure', () => {
    withI18n(<FlagBadges flags={['gps_far', 'brand_new_flag']} />)
    const btn = screen.getByRole('button', { name: /Jauh dari titik/ })
    expect(btn).toHaveAttribute('aria-expanded', 'false')
    fireEvent.click(btn)
    expect(btn).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText(/radius yang diizinkan/)).toBeInTheDocument()
    // Unknown flags are humanised.
    expect(screen.getByRole('button', { name: /Brand new flag/ })).toBeInTheDocument()
  })

  it('renders nothing without flags', () => {
    const { container } = withI18n(<FlagBadges flags={[]} />)
    expect(container).toBeEmptyDOMElement()
  })
})

describe('Popover (disclosure)', () => {
  it('closes on Escape and returns focus to the trigger', () => {
    withI18n(
      <Popover label="Menu" trigger={({ toggle, open: _o, ...aria }) => <button onClick={toggle} {...aria}>Open</button>}>
        {() => <button>Inside</button>}
      </Popover>
    )
    const trigger = screen.getByRole('button', { name: 'Open' })
    fireEvent.click(trigger)
    expect(screen.getByRole('group', { name: 'Menu' })).toBeInTheDocument()
    expect(screen.queryByRole('menu')).toBeNull()
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByRole('group', { name: 'Menu' })).toBeNull()
    expect(trigger).toHaveFocus()
  })
})

describe('Tabs', () => {
  it('links the active tab and its panel', () => {
    const onChange = vi.fn()
    withI18n(
      <>
        <Tabs idBase="t" label="Sections" active="a" onChange={onChange} tabs={[{ key: 'a', label: 'A' }, { key: 'b', label: 'B' }]} />
        <TabPanel idBase="t" active="a">
          content
        </TabPanel>
      </>
    )
    const tab = screen.getByRole('tab', { name: 'A' })
    const panel = screen.getByRole('tabpanel', { name: 'A' })
    expect(tab).toHaveAttribute('aria-controls', panel.id)
    fireEvent.keyDown(tab, { key: 'ArrowRight' })
    expect(onChange).toHaveBeenCalledWith('b')
  })
})

describe('ErrorBoundary (WEB-8)', () => {
  it('shows a friendly message, not the raw error, and resets on navigation', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    // jsdom re-reports errors React caught; keep the test output clean.
    const quiet = (e: ErrorEvent) => e.preventDefault()
    window.addEventListener('error', quiet)
    const Boom = () => {
      throw new Error('SECRET stack detail')
    }
    const { rerender } = withI18n(
      <ErrorBoundary resetKey="/a">
        <Boom />
      </ErrorBoundary>
    )
    expect(screen.getByRole('alert')).toHaveTextContent('Terjadi kesalahan')
    expect(screen.queryByText(/SECRET/)).toBeNull()
    rerender(
      <I18nProvider>
        <ErrorBoundary resetKey="/b">
          <p>fine</p>
        </ErrorBoundary>
      </I18nProvider>
    )
    expect(screen.getByText('fine')).toBeInTheDocument()
    window.removeEventListener('error', quiet)
    spy.mockRestore()
  })
})

describe('IdleLogout (WEB-4)', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('warns, ignores background time, and signs out when the countdown ends', () => {
    vi.useFakeTimers()
    const onExpire = vi.fn()
    withI18n(<IdleLogout minutes={2} onExpire={onExpire} />)
    act(() => {
      vi.advanceTimersByTime(60_000 + 1000)
    })
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByRole('timer')).toHaveTextContent(/detik/)
    act(() => {
      vi.advanceTimersByTime(60_000)
    })
    expect(onExpire).toHaveBeenCalled()
  })

  it('"Stay signed in" resets the timer; input before the warning counts as activity', () => {
    vi.useFakeTimers()
    const onExpire = vi.fn()
    withI18n(<IdleLogout minutes={2} onExpire={onExpire} />)
    act(() => {
      vi.advanceTimersByTime(50_000)
    })
    fireEvent.keyDown(window, { key: 'a' })
    act(() => {
      vi.advanceTimersByTime(50_000)
    })
    expect(screen.queryByRole('dialog')).toBeNull()
    act(() => {
      vi.advanceTimersByTime(20_000)
    })
    fireEvent.click(screen.getByRole('button', { name: /Tetap masuk/ }))
    expect(screen.queryByRole('dialog')).toBeNull()
    act(() => {
      vi.advanceTimersByTime(50_000)
    })
    expect(onExpire).not.toHaveBeenCalled()
  })
})
