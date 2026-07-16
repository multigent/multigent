import { AlertTriangle, X } from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'

type Props = {
  open: boolean
  title: string
  description: ReactNode
  confirmLabel: string
  cancelLabel: string
  busy?: boolean
  tone?: 'danger' | 'default'
  onCancel: () => void
  onConfirm: () => void
}

type ConfirmRequest = {
  title: string
  description: ReactNode
  confirmLabel: string
  cancelLabel: string
  tone?: 'danger' | 'default'
  resolve: (confirmed: boolean) => void
}

const listeners = new Set<(request: ConfirmRequest) => void>()

export function confirmDialog(options: Omit<ConfirmRequest, 'resolve'>): Promise<boolean> {
  return new Promise((resolve) => {
    const request: ConfirmRequest = { ...options, resolve }
    listeners.forEach((listener) => listener(request))
  })
}

export function ConfirmDialogHost() {
  const [request, setRequest] = useState<ConfirmRequest | null>(null)

  useEffect(() => {
    const listener = (next: ConfirmRequest) => setRequest(next)
    listeners.add(listener)
    return () => {
      listeners.delete(listener)
    }
  }, [])

  return (
    <ConfirmDialog
      open={request != null}
      title={request?.title ?? ''}
      description={request?.description ?? ''}
      confirmLabel={request?.confirmLabel ?? ''}
      cancelLabel={request?.cancelLabel ?? ''}
      tone={request?.tone}
      onCancel={() => {
        request?.resolve(false)
        setRequest(null)
      }}
      onConfirm={() => {
        request?.resolve(true)
        setRequest(null)
      }}
    />
  )
}

export function ConfirmDialog({
  open,
  title,
  description,
  confirmLabel,
  cancelLabel,
  busy = false,
  tone = 'danger',
  onCancel,
  onConfirm,
}: Props) {
  if (!open) return null
  const danger = tone === 'danger'

  return (
    <div
      className="fixed inset-0 z-[80] flex items-center justify-center bg-black/45 p-4 backdrop-blur-[1px]"
      role="presentation"
      onClick={() => !busy && onCancel()}
    >
      <div
        className="w-full max-w-md animate-scale-in rounded-xl border border-neutral-200 bg-white shadow-xl dark:border-zinc-700 dark:bg-zinc-900"
        role="dialog"
        aria-modal="true"
        aria-labelledby="confirm-dialog-title"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-start gap-3 border-b border-neutral-100 px-5 py-4 dark:border-zinc-800">
          <div className={danger ? 'mt-0.5 rounded-lg bg-red-50 p-2 text-red-600 dark:bg-red-950/40 dark:text-red-300' : 'mt-0.5 rounded-lg bg-sky-50 p-2 text-sky-600 dark:bg-sky-950/40 dark:text-sky-300'}>
            <AlertTriangle className="size-4" strokeWidth={1.8} />
          </div>
          <div className="min-w-0 flex-1">
            <h2 id="confirm-dialog-title" className="text-base font-semibold text-neutral-900 dark:text-zinc-100">
              {title}
            </h2>
            <div className="mt-1 text-sm leading-relaxed text-neutral-500 dark:text-zinc-400">
              {description}
            </div>
          </div>
          <button
            type="button"
            onClick={onCancel}
            disabled={busy}
            className="rounded-md p-1 text-neutral-400 transition-colors hover:bg-neutral-100 hover:text-neutral-700 disabled:opacity-50 dark:hover:bg-zinc-800 dark:hover:text-zinc-200"
            aria-label={cancelLabel}
          >
            <X className="size-4" strokeWidth={2} />
          </button>
        </div>
        <div className="flex justify-end gap-2 px-5 py-4">
          <button
            type="button"
            onClick={onCancel}
            disabled={busy}
            className="rounded-lg border border-neutral-300 bg-white px-3 py-2 text-sm font-medium text-neutral-700 transition-colors hover:bg-neutral-50 disabled:opacity-50 dark:border-zinc-600 dark:bg-zinc-900 dark:text-zinc-200 dark:hover:bg-zinc-800"
          >
            {cancelLabel}
          </button>
          <button
            type="button"
            onClick={onConfirm}
            disabled={busy}
            className={danger
              ? 'rounded-lg bg-red-600 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-red-700 disabled:opacity-50'
              : 'rounded-lg bg-sky-600 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-sky-700 disabled:opacity-50'}
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}
