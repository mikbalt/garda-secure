import React from 'react'
import ReactDOM from 'react-dom/client'
import { App } from './App'
import { initKeycloak } from './keycloak'
import './index.css'

initKeycloak()
  .then((authenticated) => {
    if (!authenticated) {
      // initKeycloak with onLoad:'login-required' handles redirect itself;
      // this branch is a safety fallback.
      console.error('Keycloak: not authenticated')
      return
    }

    ReactDOM.createRoot(document.getElementById('root')!).render(
      <React.StrictMode>
        <App />
      </React.StrictMode>
    )
  })
  .catch((err) => {
    document.body.innerHTML = `<div style="padding:2rem;font-family:sans-serif">
      <h2>Authentication Error</h2>
      <p>${err?.message ?? 'Failed to connect to Keycloak'}</p>
    </div>`
  })
