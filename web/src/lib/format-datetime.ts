import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'

export function normalizeDateTimeLocale(language: string | undefined): string {
  const lng = (language || 'en').toLowerCase()
  if (lng.startsWith('zh-tw') || lng.startsWith('zh-hk')) return 'zh-TW'
  if (lng.startsWith('zh')) return 'zh-CN'
  if (lng.startsWith('ja')) return 'ja-JP'
  return 'en-US'
}

export function formatDateTimeForLanguage(
  input: string | number | Date | null | undefined,
  language: string | undefined,
): string {
  if (input == null || input === '') {
    return '—'
  }
  const d = input instanceof Date ? input : new Date(input)
  if (Number.isNaN(d.getTime())) {
    return String(input)
  }
  return new Intl.DateTimeFormat(normalizeDateTimeLocale(language), {
    dateStyle: 'short',
    timeStyle: 'medium',
  }).format(d)
}

/** Formats an ISO / timestamp using the active i18n language. */
export function useFormatDateTime() {
  const { i18n } = useTranslation()

  return useCallback(
    (input: string | number | Date | null | undefined) => {
      return formatDateTimeForLanguage(input, i18n.resolvedLanguage || i18n.language)
    },
    [i18n.language, i18n.resolvedLanguage],
  )
}
