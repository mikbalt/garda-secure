import { Component, type ContextType, type ErrorInfo, type ReactNode } from 'react'
import { AlertTriangle } from 'lucide-react'
import { I18nContext } from '../i18n/context'

interface Props {
  children: ReactNode
}

interface State {
  hasError: boolean
  message: string
}

export class ErrorBoundary extends Component<Props, State> {
  static contextType = I18nContext
  declare context: ContextType<typeof I18nContext>
  state: State = { hasError: false, message: '' }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, message: error.message }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('ErrorBoundary caught:', error, info)
  }

  render() {
    if (this.state.hasError) {
      const { t } = this.context
      return (
        <div role="alert" className="flex h-full flex-col items-center justify-center py-24 text-center">
          <AlertTriangle className="mb-4 h-12 w-12 text-yellow-600" aria-hidden="true" />
          <h2 className="mb-2 text-xl font-semibold text-gray-900">{t('error.title')}</h2>
          <p className="max-w-md text-sm text-gray-700">{this.state.message}</p>
          <button
            type="button"
            className="mt-4 rounded bg-primary-800 px-4 py-2 text-white hover:bg-primary-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-600 focus-visible:ring-offset-2"
            onClick={() => this.setState({ hasError: false, message: '' })}
          >
            {t('app.retry')}
          </button>
        </div>
      )
    }
    return this.props.children
  }
}
