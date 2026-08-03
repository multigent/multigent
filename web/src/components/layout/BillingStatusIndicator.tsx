import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '../../lib/api'
import { isTrustedProxyMode, isWorkspaceAdmin, useAuth } from '../../lib/auth'
import { cn } from '../../lib/cn'
import { useFormatDateTime } from '../../lib/format-datetime'

type BillingEntitlements = {
  planCode?: string
  billingStatus?: string
  trialEndsAt?: string
}

type BillingEntitlementsResp = {
  entitlements: BillingEntitlements
}

export function BillingStatusIndicator() {
  const { t } = useTranslation()
  const { user } = useAuth()
  const formatDateTime = useFormatDateTime()
  const [entitlements, setEntitlements] = useState<BillingEntitlements | null>(null)
  const canSeeBilling = isTrustedProxyMode() && isWorkspaceAdmin(user)

  const load = useCallback(() => {
    if (!canSeeBilling) {
      setEntitlements(null)
      return
    }
    apiFetch<BillingEntitlementsResp>('/api/v1/billing/entitlements', { suppressToast: true })
      .then((resp) => setEntitlements(resp.entitlements || null))
      .catch(() => setEntitlements(null))
  }, [canSeeBilling])

  useEffect(() => {
    load()
    window.addEventListener('workspace-changed', load)
    return () => window.removeEventListener('workspace-changed', load)
  }, [load])

  if (!canSeeBilling || !entitlements) return null

  const status = entitlements.billingStatus || 'unlimited'
  const plan = entitlements.planCode || ''
  const trialEndsAt = entitlements.trialEndsAt
  const daysLeft = trialEndsAt ? Math.max(0, Math.ceil((new Date(trialEndsAt).getTime() - Date.now()) / 86400000)) : null
  const planName = plan && plan !== 'trial'
    ? t(`settings.billingPlan_${plan}`, { defaultValue: plan.charAt(0).toUpperCase() + plan.slice(1) })
    : ''
  const planLabel = planName ? t('settings.billingPlanLabel', { plan: planName }) : ''
  const statusLabel = t(`settings.billingStatus_${status}`, { defaultValue: status })
  const isTrial = status === 'trialing'
  const label = isTrial
    ? t('settings.billingTrialPill', { count: daysLeft ?? 0 })
    : (planLabel || statusLabel)
  const title = [
    planLabel || statusLabel,
    trialEndsAt ? t('settings.billingTrialEndsAt', { date: formatDateTime(trialEndsAt) }) : '',
  ].filter(Boolean).join(' · ')
  const isAttention = status === 'trial_expired' || status === 'past_due' || status === 'canceled'
  const isStarter = status === 'active' && plan === 'starter'
  const isBusiness = status === 'active' && plan === 'business'

  return (
    <Link
      to="/settings"
      className={cn(
        'inline-flex min-w-0 items-center rounded-md px-2 py-0.5 text-xs text-neutral-400 transition-colors hover:bg-neutral-100 hover:text-neutral-600 dark:text-zinc-500 dark:hover:bg-zinc-800 dark:hover:text-zinc-300',
        isStarter && 'text-slate-500 hover:text-slate-700 dark:text-zinc-400 dark:hover:text-zinc-200',
        isBusiness && 'text-indigo-500/80 hover:text-indigo-600 dark:text-indigo-300/80 dark:hover:text-indigo-200',
        isAttention && 'text-amber-600 hover:text-amber-700 dark:text-amber-400 dark:hover:text-amber-300',
      )}
      title={title || label}
    >
      <span className="truncate">{label}</span>
    </Link>
  )
}
