import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import './i18n'
import '@xyflow/react/dist/style.css'
import './index.css'
import App from './App.tsx'
import { AuthProvider } from './lib/auth'
import { ThemeProvider } from './theme/ThemeProvider'

function routerBasename() {
  if (!hasTrustedProxyCookie()) return undefined
  const bare = /^\/([a-z0-9][a-z0-9-]*)(\/|$)/.exec(window.location.pathname)
  if (bare && !appRouteSegments.has(bare[1])) return `/${bare[1]}`
  return undefined
}

function hasTrustedProxyCookie() {
  return typeof document !== 'undefined' && document.cookie.split(';').some((item) => item.trim() === 'mg_trusted_proxy=1')
}

const appRouteSegments = new Set([
  'account',
  'audit',
  'connections',
  'docs',
  'files',
  'goals',
  'invite',
  'login',
  'playbooks',
  'projects',
  'settings',
  'skills',
  'teams',
  'users',
  'workbench',
  'workflows',
])

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ThemeProvider>
      <AuthProvider>
        <BrowserRouter basename={routerBasename()}>
          <App />
        </BrowserRouter>
      </AuthProvider>
    </ThemeProvider>
  </StrictMode>,
)
