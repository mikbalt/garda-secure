import React from 'react'
import ReactDOM from 'react-dom/client'
import { I18nProvider } from './i18n/I18nProvider'
import { Bootstrap } from './bootstrap/Bootstrap'
import './index.css'

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <I18nProvider>
      <Bootstrap />
    </I18nProvider>
  </React.StrictMode>
)
