import { Check, ChevronDown, X } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '../../lib/cn'
import { STATUS_KEYS, type TaskStatus } from './TaskModals'

type Props = {
  value: string[]
  onChange: (value: string[]) => void
  className?: string
}

export function TaskStatusFilter({ value, onChange, className }: Props) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement | null>(null)
  const selected = new Set(value)

  useEffect(() => {
    if (!open) return
    function onPointerDown(event: PointerEvent) {
      if (ref.current && !ref.current.contains(event.target as Node)) setOpen(false)
    }
    window.addEventListener('pointerdown', onPointerDown)
    return () => window.removeEventListener('pointerdown', onPointerDown)
  }, [open])

  function toggle(status: TaskStatus) {
    const next = new Set(selected)
    if (next.has(status)) next.delete(status)
    else next.add(status)
    onChange(STATUS_KEYS.filter((key) => next.has(key)))
  }

  const label = value.length === 0
    ? `${t('tasks.filterStatus')}: ${t('messages.readAll')}`
    : value.length === 1
      ? t(`tasks.status.${value[0]}`)
      : `${t('tasks.filterStatus')}: ${value.length}`

  return (
    <div ref={ref} className={cn('relative', className)}>
      <button
        type="button"
        onClick={() => setOpen((current) => !current)}
        className="flex h-8 min-w-40 items-center justify-between gap-2 rounded-md border border-neutral-200/80 bg-white px-2.5 text-[13px] text-neutral-700 outline-none transition-colors hover:border-neutral-300 focus:border-sky-400 dark:border-zinc-700/60 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:border-zinc-600"
      >
        <span className="truncate">{label}</span>
        <ChevronDown className={cn('size-3.5 shrink-0 text-neutral-400 transition-transform dark:text-zinc-500', open && 'rotate-180')} strokeWidth={1.8} />
      </button>
      {open && (
        <div className="absolute left-0 top-full z-30 mt-1 w-60 rounded-lg border border-neutral-200 bg-white p-1.5 shadow-lg dark:border-zinc-700 dark:bg-zinc-900">
          <button
            type="button"
            onClick={() => onChange([])}
            className="mb-1 flex w-full items-center justify-between rounded-md px-2 py-1.5 text-left text-[13px] text-neutral-500 transition-colors hover:bg-neutral-50 hover:text-neutral-800 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
          >
            <span>{t('messages.readAll')}</span>
            {value.length === 0 && <Check className="size-3.5 text-sky-600 dark:text-sky-400" strokeWidth={2} />}
          </button>
          <div className="my-1 h-px bg-neutral-100 dark:bg-zinc-800" />
          {STATUS_KEYS.map((status) => {
            const active = selected.has(status)
            return (
              <button
                key={status}
                type="button"
                onClick={() => toggle(status)}
                className={cn(
                  'flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-[13px] transition-colors',
                  active
                    ? 'bg-sky-50 text-sky-800 dark:bg-sky-950/40 dark:text-sky-300'
                    : 'text-neutral-600 hover:bg-neutral-50 hover:text-neutral-900 dark:text-zinc-300 dark:hover:bg-zinc-800 dark:hover:text-zinc-100',
                )}
              >
                <span className={cn('flex size-3.5 items-center justify-center rounded border', active ? 'border-sky-500 bg-sky-500 text-white' : 'border-neutral-300 dark:border-zinc-600')}>
                  {active && <Check className="size-3" strokeWidth={2.2} />}
                </span>
                <span>{t(`tasks.status.${status}`)}</span>
              </button>
            )
          })}
          {value.length > 0 && (
            <button
              type="button"
              onClick={() => onChange([])}
              className="mt-1 flex w-full items-center gap-1.5 rounded-md px-2 py-1.5 text-left text-[12px] text-neutral-400 transition-colors hover:bg-neutral-50 hover:text-neutral-700 dark:text-zinc-500 dark:hover:bg-zinc-800 dark:hover:text-zinc-300"
            >
              <X className="size-3" strokeWidth={2} />
              {t('messages.resetFilters')}
            </button>
          )}
        </div>
      )}
    </div>
  )
}
