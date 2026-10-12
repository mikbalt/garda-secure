import { Component, type ContextType, type ErrorInfo, type ReactNode } from 'react'
import { AlertTriangle } from 'lucide-react'
import { I18nContext } from '../i18n/context'
import { requestIdText } from '../i18n/errors'

interface Props {
  children: ReactNode
  /** When this value changes (e.g. the route path), a caught error is cleared. */
  resetKey?: string
}

interface State {
  error: unknown
  hasError: boolean
}

/**
 * Catches render errors (WEB-8). The app keeps one boundary around the shell
 * and one per route inside the Layout (keyed by the pathname), so a broken
 * page leaves the navigation usable and recovers on navigation. Shows a
 * friendly message and, for API errors, the request id for support — never
 * the raw error text.
 */
export class ErrorBoundary extends Component<Props, State> {
  static contextType = I18nContext
  declare context: ContextType<typeof I18nContext>
  state: State = { error: null, hasError: false }

  static getDerivedStateFromError(error: unknown): State {
    return { error, hasError: true }
  }

  componentDidCatch(error: unknown, info: ErrorInfo) {
    console.error('ErrorBoundary caught:', error, info.componentStack)
  }

  componentDidUpdate(prev: Props) {
    if (this.state.hasError && prev.resetKey !== this.props.resetKey) this.setState({ error: null, hasError: false })
  }

  render() {
    if (!this.state.hasError) return this.props.children
    const { t, lang } = this.context
    const ref = requestIdText(this.state.error, lang)
    return (
      <div className="flex h-full flex-col items-center justify-center px-4 py-24 text-center">
        <AlertTriangle className="mb-4 h-12 w-12 text-amber-600" aria-hidden="true" />
        <div role="alert">
          <h2 className="mb-2 text-xl font-semibold text-gray-900">{t('error.title')}</h2>
          <p className="max-w-md text-sm text-gray-700">{t('error.body')}</p>
          {ref && <p className="mt-2 font-mono text-xs text-gray-600">{ref}</p>}
        </div>
        <button
          type="button"
          className="mt-5 rounded-lg bg-primary-600 px-4 py-2 text-sm font-medium text-white hover:bg-primary-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-600 focus-visible:ring-offset-2"
          onClick={() => this.setState({ error: null, hasError: false })}
        >
          {t('app.retry')}
        </button>
      </div>
    )
  }
}
