import { useEffect, useRef, useState } from 'react'
import { apiFetch } from './api'

export type ApiState<T> =
  | { status: 'loading' }
  | { status: 'error'; error: Error }
  | { status: 'ok'; data: T }

type UseApiJsonOptions = {
  silentStatuses?: number[]
}

export function useApiJson<T>(path: string | null, reloadKey = 0, options?: UseApiJsonOptions): ApiState<T> {
  const [state, setState] = useState<ApiState<T>>({ status: 'loading' })
  const prevPath = useRef(path)
  const prevReloadKey = useRef(reloadKey)
  const silentStatuses = options?.silentStatuses

  useEffect(() => {
    if (path == null) {
      return
    }
    let cancelled = false
    const pathChanged = prevPath.current !== path
    const reloadChanged = prevReloadKey.current !== reloadKey
    prevPath.current = path
    prevReloadKey.current = reloadKey
    if (pathChanged || reloadChanged) {
      setState({ status: 'loading' })
    }
    const url = reloadKey ? `${path}${path.includes('?') ? '&' : '?'}_=${reloadKey}` : path
    apiFetch<T>(url, silentStatuses ? { silentStatuses } : undefined)
      .then((data) => {
        if (!cancelled) {
          setState({ status: 'ok', data })
        }
      })
      .catch((e) => {
        if (!cancelled) {
          setState({
            status: 'error',
            error: e instanceof Error ? e : new Error(String(e)),
          })
        }
      })
    return () => {
      cancelled = true
    }
  }, [path, reloadKey, silentStatuses])

  if (path == null) {
    return { status: 'error', error: new Error('no path') }
  }
  return state
}
