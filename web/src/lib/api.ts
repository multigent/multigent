import { getStoredToken } from './auth'
import { showToast } from '../components/ui/Toast'
import { i18n } from '../i18n'

type APIErrorBody = {
  code?: string
  message?: string
  details?: Record<string, unknown>
  requestId?: string
}

export class ApiError extends Error {
  status: number
  code: string
  requestId?: string
  details?: Record<string, unknown>
  serverMessage: string

  constructor(status: number, body: APIErrorBody) {
    const code = body.code || fallbackCodeForStatus(status)
    const serverMessage = body.message || ''
    super(localizedAPIErrorMessage(code, serverMessage || `${status}`))
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.requestId = body.requestId
    this.details = body.details
    this.serverMessage = serverMessage
  }
}

export function apiBase(): string {
  return (import.meta.env.VITE_API_BASE ?? '').replace(/\/$/, '')
}

export function apiUrl(path: string): string {
  const p = path.startsWith('/') ? path : `/${path}`
  const b = apiBase()
  return b ? `${b}${p}` : p
}

function authHeaders(extra?: HeadersInit): Headers {
  const headers = new Headers(extra)
  const token = getStoredToken()
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }
  return headers
}

function handle401() {
  localStorage.removeItem('multigent-token')
  localStorage.removeItem('multigent-user')
  window.dispatchEvent(new Event('auth-expired'))
}

async function handleResponse<T>(res: Response): Promise<T> {
  if (res.status === 401) {
    handle401()
    throw new ApiError(res.status, { code: 'unauthorized', message: 'unauthorized' })
  }
  if (!res.ok) {
    const err = await parseAPIError(res)
    showToast(err.message, 'error')
    throw err
  }
  if (res.status === 204) return undefined as T
  const ct = res.headers.get('content-type') ?? ''
  if (ct.includes('application/json')) return res.json() as Promise<T>
  return undefined as T
}

export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = authHeaders(init?.headers)
  if (!headers.has('Accept')) headers.set('Accept', 'application/json')
  const res = await fetch(apiUrl(path), { ...init, headers })
  return handleResponse<T>(res)
}

export async function apiPost<T>(path: string, body: unknown): Promise<T> {
  const headers = authHeaders()
  headers.set('Content-Type', 'application/json')
  headers.set('Accept', 'application/json')
  const res = await fetch(apiUrl(path), { method: 'POST', headers, body: JSON.stringify(body) })
  return handleResponse<T>(res)
}

export async function apiPatch<T>(path: string, body: unknown): Promise<T> {
  const headers = authHeaders()
  headers.set('Content-Type', 'application/json')
  headers.set('Accept', 'application/json')
  const res = await fetch(apiUrl(path), { method: 'PATCH', headers, body: JSON.stringify(body) })
  return handleResponse<T>(res)
}

export async function apiPut<T>(path: string, body: unknown): Promise<T> {
  const headers = authHeaders()
  headers.set('Content-Type', 'application/json')
  headers.set('Accept', 'application/json')
  const res = await fetch(apiUrl(path), { method: 'PUT', headers, body: JSON.stringify(body) })
  return handleResponse<T>(res)
}

export async function apiDelete(path: string): Promise<void> {
  const headers = authHeaders()
  headers.set('Accept', 'application/json')
  const res = await fetch(apiUrl(path), { method: 'DELETE', headers })
  await handleResponse<void>(res)
}

/** No-auth POST for login */
export async function apiPublicFetch<T>(path: string): Promise<T> {
  const headers = new Headers()
  headers.set('Accept', 'application/json')
  const res = await fetch(apiUrl(path), { headers })
  return handleResponse<T>(res)
}

export async function apiLoginPost<T>(path: string, body: unknown): Promise<T> {
  const headers = new Headers()
  headers.set('Content-Type', 'application/json')
  headers.set('Accept', 'application/json')
  const res = await fetch(apiUrl(path), { method: 'POST', headers, body: JSON.stringify(body) })
  if (!res.ok) {
    throw await parseAPIError(res)
  }
  return res.json() as Promise<T>
}

export function apiTeamPath(teamId: string): string {
  return teamId.split('/').filter(Boolean).map((s) => encodeURIComponent(s)).join('/')
}

async function parseAPIError(res: Response): Promise<ApiError> {
  const text = await res.text()
  const fallbackMessage = text.trim().slice(0, 200) || res.statusText
  try {
    const parsed = JSON.parse(text) as { error?: string | APIErrorBody; code?: string; message?: string; details?: Record<string, unknown>; requestId?: string }
    if (parsed && typeof parsed.error === 'object' && parsed.error !== null) {
      return new ApiError(res.status, parsed.error)
    }
    if (parsed && typeof parsed.error === 'string') {
      return new ApiError(res.status, {
        code: parsed.code || fallbackCodeForStatus(res.status),
        message: parsed.error,
        details: parsed.details,
        requestId: parsed.requestId,
      })
    }
    if (parsed && (parsed.code || parsed.message)) {
      return new ApiError(res.status, {
        code: parsed.code,
        message: parsed.message || fallbackMessage,
        details: parsed.details,
        requestId: parsed.requestId,
      })
    }
  } catch { /* plain text */ }
  return new ApiError(res.status, { code: fallbackCodeForStatus(res.status), message: fallbackMessage })
}

function localizedAPIErrorMessage(code: string, fallback: string): string {
  const key = `apiErrors.${code}`
  const translated = i18n.t(key)
  if (translated && translated !== key) return translated
  return fallback || i18n.t('apiErrors.unknown')
}

function fallbackCodeForStatus(status: number): string {
  switch (status) {
    case 400: return 'bad_request'
    case 401: return 'unauthorized'
    case 403: return 'forbidden'
    case 404: return 'not_found'
    case 409: return 'conflict'
    case 502: return 'upstream_error'
    case 503: return 'service_unavailable'
    default: return status >= 500 ? 'internal_error' : 'bad_request'
  }
}
