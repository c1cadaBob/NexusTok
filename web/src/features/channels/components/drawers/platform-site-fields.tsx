import { useMutation, useQuery } from '@tanstack/react-query'
import {
  ExternalLink,
  KeyRound,
  Loader2,
  RefreshCw,
  ShieldCheck,
} from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { CopyButton } from '@/components/copy-button'
import { Button } from '@/components/ui/button'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

import {
  cancelPlatformSiteAuthFlow,
  completePlatformSiteCapture,
  getPlatformSiteCaptureStatus,
  startPlatformSiteAuthFlow,
  startPlatformSiteCapture,
  verifyPlatformSiteAuthFlow,
} from '../../api'
import { platformForChannelType, type ChannelFormValues } from '../../lib'
import type {
  PlatformSiteCaptureCompletePayload,
  UpstreamSiteStatus,
} from '../../types'

type PlatformSiteFieldsProps = {
  disabled: boolean
  isEditing: boolean
  channelId?: number
  syncStatus?: UpstreamSiteStatus
  onCaptureCompleted?: (captureID: string) => void
}

const RATIO_COMPARE_EPSILON = 0.0000001
const RATIO_INPUT_PRECISION = 1000000

function openPlatformCaptureWindow(
  url: string,
  targetWindow?: Window | null
): Window | null {
  const targetURL = url.trim()
  if (!targetURL) return null
  try {
    if (targetWindow && !targetWindow.closed) {
      targetWindow.location.href = targetURL
      targetWindow.focus()
      return targetWindow
    }
  } catch {
    // 复用预打开窗口失败时继续尝试创建新标签页。
  }
  try {
    const opened = window.open(targetURL, '_blank')
    if (!opened) return null
    opened.focus()
    return opened
  } catch {
    return null
  }
}

function normalizePlatformNumber(
  value: number | null | undefined,
  fallback = 0
): number {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) {
    return fallback
  }
  return value
}

function normalizePlatformRatio(value: number): number {
  return Math.round(value * RATIO_INPUT_PRECISION) / RATIO_INPUT_PRECISION
}

function formatPlatformCaptureAuthType(
  value: string | undefined,
  t: (key: string) => string
): string {
  return value === 'password'
    ? t('Username and password')
    : t('Automatic configuration')
}

function formatPlatformRatioInput(value: number): string {
  return String(normalizePlatformRatio(normalizePlatformNumber(value, 1)))
}

function isCaptureSessionExpiredMessage(value: string | undefined): boolean {
  const message = (value || '').toLowerCase()
  return (
    message.includes('采集会话不存在') ||
    message.includes('采集会话已过期') ||
    (message.includes('capture session') && message.includes('expired')) ||
    (message.includes('capture session') && message.includes('not found'))
  )
}

function isPlatformSiteCapturePayload(
  value: unknown
): value is PlatformSiteCaptureCompletePayload {
  if (!value || typeof value !== 'object') return false
  const payload = value as Partial<PlatformSiteCaptureCompletePayload>
  return (
    typeof payload.capture_secret === 'string' &&
    Boolean(payload.capture_secret.trim())
  )
}

function createCaptureBridgeBootstrap(
  captureID: string,
  openerOrigin: string
): string {
  const encodedCaptureID = JSON.stringify(captureID)
  const encodedOpenerOrigin = JSON.stringify(openerOrigin)
  return `(()=>{const o=window.opener;if(!o||o.closed)return;const c=${encodedCaptureID};const t=${encodedOpenerOrigin};let timer=0;const cleanup=()=>{window.removeEventListener('message',onMessage);if(timer)window.clearTimeout(timer)};const onMessage=e=>{const d=e.data||{};if(e.source!==o||e.origin!==t||d.type!=='nexustok-upstream-capture-bridge-script'||String(d.capture_id||d.captureID||'')!==c||typeof d.script!=='string'||!d.script)return;cleanup();const s=document.createElement('script');const n=Array.from(document.scripts).find((item)=>item&&item.nonce);if(n&&n.nonce)s.nonce=n.nonce;s.textContent=d.script;(document.head||document.documentElement).appendChild(s)};window.addEventListener('message',onMessage);timer=window.setTimeout(cleanup,15000);o.postMessage({type:'nexustok-upstream-capture-bridge-script-request',capture_id:c,captureID:c},t)})()`
}

export function PlatformSiteFields(props: PlatformSiteFieldsProps) {
  const { t } = useTranslation()
  const form = useFormContext<ChannelFormValues>()
  const channelType = useWatch({
    control: form.control,
    name: 'type',
  })
  const platform = platformForChannelType(channelType ?? 0)
  const authType = useWatch({
    control: form.control,
    name: 'platform_site_auth_type',
  })
  const baseURL = useWatch({
    control: form.control,
    name: 'base_url',
  })
  const captureID = useWatch({
    control: form.control,
    name: 'platform_site_capture_id',
  })
  const authFlowID = useWatch({
    control: form.control,
    name: 'platform_site_auth_flow_id',
  })
  const username = useWatch({
    control: form.control,
    name: 'platform_site_username',
  })
  const password = useWatch({
    control: form.control,
    name: 'platform_site_password',
  })
  const watchedRechargeAmount = useWatch({
    control: form.control,
    name: 'platform_site_recharge_amount',
  })
  const watchedCreditedAmount = useWatch({
    control: form.control,
    name: 'platform_site_credited_amount',
  })
  const watchedConversionRatio = useWatch({
    control: form.control,
    name: 'platform_site_conversion_ratio',
  })
  const watchedRatioIsOverridden = useWatch({
    control: form.control,
    name: 'platform_site_conversion_ratio_override',
  })
  const rechargeAmount = normalizePlatformNumber(watchedRechargeAmount)
  const creditedAmount = normalizePlatformNumber(watchedCreditedAmount)
  const conversionRatio = normalizePlatformNumber(watchedConversionRatio, 1)
  const previewRatio =
    creditedAmount > 0
      ? normalizePlatformRatio(rechargeAmount / creditedAmount)
      : undefined
  const ratioIsOverridden = watchedRatioIsOverridden === true
  const completedCaptureRef = useRef('')
  const pendingCaptureWindowRef = useRef<Window | null>(null)
  const activeCaptureWindowRef = useRef<Window | null>(null)
  const bridgeScriptRef = useRef('')
  const bridgeCompletionRef = useRef(false)
  const [captureOrigin, setCaptureOrigin] = useState('')
  const [captureRequiredHelperVersion, setCaptureRequiredHelperVersion] =
    useState('')
  const [handoffURL, setHandoffURL] = useState('')
  const [bridgeURL, setBridgeURL] = useState('')
  const [bridgeBookmarklet, setBridgeBookmarklet] = useState('')
  const [bridgeLoading, setBridgeLoading] = useState(false)
  const [helperInstallURL, setHelperInstallURL] = useState('')
  const [showHandoffFallback, setShowHandoffFallback] = useState(false)
  const [captureSessionExpired, setCaptureSessionExpired] = useState(false)
  const [helperVersion, setHelperVersion] = useState('')
  const [helperDetection, setHelperDetection] = useState<
    'idle' | 'probing' | 'ready' | 'missing' | 'outdated'
  >('idle')
  const [twoFactorCode, setTwoFactorCode] = useState('')
  const [authFlowStatus, setAuthFlowStatus] = useState<
    'two_factor_required' | 'authenticated' | undefined
  >()
  const authFlowRef = useRef('')
  const helperProbeTimerRef = useRef<number | null>(null)

  const captureStatusQuery = useQuery({
    queryKey: ['platform-site-capture-status', captureID],
    queryFn: () => getPlatformSiteCaptureStatus(captureID || ''),
    enabled: Boolean(captureID) && !captureSessionExpired,
    refetchInterval: (query) => {
      const response = query.state.data
      if (!response?.success) return false
      const status = response.data?.status
      return status === 'completed' || status === 'failed' ? false : 2000
    },
  })

  const captureMutation = useMutation({
    mutationFn: () => {
      if (authType !== 'auto') {
        throw new Error(t('Select a script-based authentication method first.'))
      }
      return startPlatformSiteCapture({
        platform,
        base_url: baseURL || '',
        auth_type: 'auto',
        channel_id: props.channelId,
        return_url: window.location.href,
      })
    },
    onSuccess: (response) => {
      if (!response.success || !response.data) {
        toast.error(response.message || t('Failed to create capture session'))
        return
      }
      form.setValue('platform_site_capture_id', response.data.capture_id, {
        shouldDirty: true,
        shouldValidate: true,
      })
      setCaptureSessionExpired(false)
      setCaptureOrigin(response.data.origin || '')
      setCaptureRequiredHelperVersion(
        response.data.helper_required_version ||
          response.data.helper_version ||
          ''
      )
      completedCaptureRef.current = ''
      bridgeCompletionRef.current = false
      setHandoffURL(response.data.handoff_url)
      setBridgeURL(response.data.capture_bridge_url || '')
      setBridgeBookmarklet('')
      bridgeScriptRef.current = ''
      setHelperInstallURL(
        response.data.helper_install_url || response.data.userscript_url || ''
      )
      setHelperVersion(response.data.helper_version || '')
      setHelperDetection('probing')
      if (helperProbeTimerRef.current) {
        window.clearTimeout(helperProbeTimerRef.current)
      }
      helperProbeTimerRef.current = window.setTimeout(() => {
        setHelperDetection('missing')
      }, 4500)
      const targetWindow = pendingCaptureWindowRef.current
      pendingCaptureWindowRef.current = null
      const hasTargetWindow = Boolean(targetWindow && !targetWindow.closed)
      if (hasTargetWindow) {
        activeCaptureWindowRef.current = targetWindow
      }
      const openedWindow = hasTargetWindow
        ? openPlatformCaptureWindow(response.data.handoff_url, targetWindow)
        : null
      const opened = Boolean(openedWindow)
      if (!opened) {
        setShowHandoffFallback(true)
        toast.info(
          t(
            'Browser blocked the upstream capture tab. Use the button below to continue.'
          )
        )
      } else {
        setShowHandoffFallback(false)
      }
      toast.success(t('Capture session created'))
    },
    onError: (error: unknown) => {
      if (
        pendingCaptureWindowRef.current &&
        !pendingCaptureWindowRef.current.closed
      ) {
        pendingCaptureWindowRef.current.close()
      }
      pendingCaptureWindowRef.current = null
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to create capture session')
      )
    },
  })

  const passwordAuthMutation = useMutation({
    mutationFn: () =>
      startPlatformSiteAuthFlow({
        platform,
        base_url: baseURL || '',
        auth_type: 'password',
        username: form.getValues('platform_site_username') || '',
        password: form.getValues('platform_site_password') || '',
        channel_id: props.channelId,
      }),
    onSuccess: (response) => {
      if (!response.success || !response.data) {
        toast.error(response.message || t('Platform login failed'))
        return
      }
      const flow = response.data
      form.setValue('platform_site_auth_flow_id', flow.flow_id, {
        shouldDirty: true,
        shouldValidate: true,
      })
      authFlowRef.current = flow.flow_id
      setAuthFlowStatus(
        flow.status === 'two_factor_required' ? flow.status : 'authenticated'
      )
      form.setValue('platform_site_password', '')
      if (flow.status === 'two_factor_required') {
        setTwoFactorCode('')
        toast.info(t('Enter the upstream two-factor code to continue'))
        return
      }
      toast.success(t('Platform login verified'))
    },
    onError: (error: unknown) => {
      toast.error(
        error instanceof Error ? error.message : t('Platform login failed')
      )
    },
  })

  const verifyAuthFlowMutation = useMutation({
    mutationFn: () =>
      verifyPlatformSiteAuthFlow(authFlowID || '', {
        code: twoFactorCode.trim(),
      }),
    onSuccess: (response) => {
      if (!response.success || !response.data) {
        toast.error(response.message || t('Two-factor verification failed'))
        return
      }
      if (response.data.status !== 'authenticated') {
        toast.error(response.message || t('Two-factor verification failed'))
        return
      }
      form.setValue('platform_site_auth_flow_id', response.data.flow_id, {
        shouldDirty: true,
        shouldValidate: true,
      })
      authFlowRef.current = response.data.flow_id
      setAuthFlowStatus('authenticated')
      setTwoFactorCode('')
      toast.success(t('Platform login verified'))
    },
    onError: (error: unknown) => {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Two-factor verification failed')
      )
    },
  })

  const captureStatusResponse = captureStatusQuery.data
  const captureStatus = captureStatusResponse?.data
  const onCaptureCompleted = props.onCaptureCompleted

  useEffect(() => {
    if (!captureStatus) return
    setCaptureOrigin(captureStatus.origin || '')
    setCaptureRequiredHelperVersion(
      captureStatus.helper_required_version ||
        captureStatus.helper_version ||
        ''
    )
  }, [
    captureStatus,
    captureStatus?.helper_required_version,
    captureStatus?.helper_version,
    captureStatus?.origin,
  ])

  useEffect(() => {
    if (
      !captureID ||
      !captureStatusResponse ||
      captureStatusResponse.success ||
      !isCaptureSessionExpiredMessage(captureStatusResponse.message)
    ) {
      return
    }
    setCaptureSessionExpired(true)
    bridgeCompletionRef.current = false
    setHandoffURL('')
    setBridgeURL('')
    setBridgeBookmarklet('')
    bridgeScriptRef.current = ''
    setCaptureOrigin('')
    setCaptureRequiredHelperVersion('')
    setShowHandoffFallback(false)
    completedCaptureRef.current = ''
    form.setValue('platform_site_capture_id', '', {
      shouldDirty: true,
      shouldValidate: true,
    })
  }, [captureID, captureStatusResponse, form])

  useEffect(() => {
    if (!captureStatusQuery.isError || !captureID) return
    const error = captureStatusQuery.error as {
      response?: { data?: { message?: string } }
      message?: string
    }
    if (
      !isCaptureSessionExpiredMessage(
        error.response?.data?.message || error.message
      )
    ) {
      return
    }
    setCaptureSessionExpired(true)
    bridgeCompletionRef.current = false
    setHandoffURL('')
    setBridgeURL('')
    setBridgeBookmarklet('')
    bridgeScriptRef.current = ''
    setCaptureOrigin('')
    setCaptureRequiredHelperVersion('')
    setShowHandoffFallback(false)
    completedCaptureRef.current = ''
    form.setValue('platform_site_capture_id', '', {
      shouldDirty: true,
      shouldValidate: true,
    })
  }, [captureID, captureStatusQuery.error, captureStatusQuery.isError, form])

  useEffect(() => {
    if (typeof window === 'undefined') return
    const currentURL = new URL(window.location.href)
    const returnedCaptureID = currentURL.searchParams
      .get('platform_site_capture_id')
      ?.trim()
    if (!returnedCaptureID) return

    if (captureID !== returnedCaptureID) {
      form.setValue('platform_site_capture_id', returnedCaptureID, {
        shouldDirty: true,
        shouldValidate: true,
      })
      completedCaptureRef.current = ''
    }
    currentURL.searchParams.delete('platform_site_capture_id')
    window.history.replaceState(
      window.history.state,
      document.title,
      currentURL.toString()
    )
  }, [captureID, form])

  useEffect(() => {
    if (!captureStatus || captureStatus.status !== 'completed') return
    if (completedCaptureRef.current === captureStatus.capture_id) return
    completedCaptureRef.current = captureStatus.capture_id
    if (helperProbeTimerRef.current) {
      window.clearTimeout(helperProbeTimerRef.current)
      helperProbeTimerRef.current = null
    }
    setHelperDetection('ready')
    setHelperVersion(
      captureStatus.helper_version ||
        captureStatus.diagnostics?.helper_version ||
        ''
    )
    setShowHandoffFallback(false)
    toast.success(t('Upstream login state captured'))
    onCaptureCompleted?.(captureStatus.capture_id)
  }, [captureStatus, onCaptureCompleted, t])

  useEffect(() => {
    const handleHelperMessage = (event: MessageEvent) => {
      const data = event.data as
        | {
            type?: string
            helper_version?: string
            helperVersion?: string
            capture_id?: string
            captureID?: string
            handoff_url?: string
            handoffURL?: string
            script?: string
            payload?: unknown
          }
        | undefined
      if (
        !data ||
        (data.type !== 'nexustok-upstream-capture-helper-ready' &&
          data.type !== 'nexustok-upstream-capture-bridge-request' &&
          data.type !== 'nexustok-upstream-capture-bridge-script-request' &&
          data.type !== 'nexustok-upstream-capture-bridge-result')
      ) {
        return
      }
      const expectedCaptureID = captureID || ''
      const messageCaptureID = String(
        data.capture_id || data.captureID || ''
      ).trim()
      const expectedOrigin =
        captureStatus?.origin?.trim() || captureOrigin.trim()
      if (expectedOrigin && event.origin !== expectedOrigin) return
      const expectedWindow = activeCaptureWindowRef.current

      if (data.type === 'nexustok-upstream-capture-bridge-request') {
        if (
          !expectedCaptureID ||
          messageCaptureID !== expectedCaptureID ||
          !expectedOrigin ||
          !expectedWindow ||
          event.source !== expectedWindow
        ) {
          return
        }
        const handoff = handoffURL || captureStatus?.handoff_url || ''
        if (!handoff) return
        const sourceWindow = event.source as Window
        sourceWindow.postMessage(
          {
            type: 'nexustok-upstream-capture-bridge-handoff',
            capture_id: expectedCaptureID,
            captureID: expectedCaptureID,
            handoff_url: handoff,
            handoffURL: handoff,
          },
          event.origin
        )
        return
      }

      if (data.type === 'nexustok-upstream-capture-bridge-script-request') {
        if (
          !expectedCaptureID ||
          messageCaptureID !== expectedCaptureID ||
          !expectedOrigin ||
          !expectedWindow ||
          event.source !== expectedWindow ||
          !bridgeScriptRef.current
        ) {
          return
        }
        const sourceWindow = event.source as Window
        sourceWindow.postMessage(
          {
            type: 'nexustok-upstream-capture-bridge-script',
            capture_id: expectedCaptureID,
            captureID: expectedCaptureID,
            script: bridgeScriptRef.current,
          },
          event.origin
        )
        return
      }

      if (messageCaptureID && messageCaptureID !== expectedCaptureID) return

      if (data.type === 'nexustok-upstream-capture-bridge-result') {
        if (
          !expectedCaptureID ||
          messageCaptureID !== expectedCaptureID ||
          !expectedOrigin ||
          event.origin !== expectedOrigin ||
          !expectedWindow ||
          event.source !== expectedWindow ||
          bridgeCompletionRef.current ||
          !isPlatformSiteCapturePayload(data.payload)
        ) {
          return
        }
        bridgeCompletionRef.current = true
        const completionPayload: PlatformSiteCaptureCompletePayload = {
          ...data.payload,
          capture_source: 'capture_bridge',
          helper_version:
            captureStatus?.helper_required_version ||
            captureStatus?.helper_version ||
            captureRequiredHelperVersion ||
            helperVersion,
          platform: captureStatus?.platform || platform,
          auth_type: 'auto',
          origin: event.origin,
        }
        void completePlatformSiteCapture(expectedCaptureID, completionPayload)
          .then((response) => {
            const sourceWindow = event.source as Window
            sourceWindow.postMessage(
              {
                type: 'nexustok-upstream-capture-bridge-ack',
                capture_id: expectedCaptureID,
                success: response.success === true,
              },
              event.origin
            )
            if (!response.success) {
              toast.error(
                response.message ||
                  t('Page bridge capture failed. Please retry.')
              )
            }
          })
          .catch(() => {
            const sourceWindow = event.source as Window
            sourceWindow.postMessage(
              {
                type: 'nexustok-upstream-capture-bridge-ack',
                capture_id: expectedCaptureID,
                success: false,
              },
              event.origin
            )
            toast.error(t('Page bridge capture failed. Please retry.'))
          })
          .finally(() => {
            bridgeCompletionRef.current = false
          })
        return
      }

      if (expectedWindow && event.source && event.source !== expectedWindow) {
        return
      }
      const detectedVersion = String(
        data.helper_version || data.helperVersion || ''
      ).trim()
      const requiredVersion = String(
        captureStatus?.helper_required_version ||
          captureStatus?.helper_version ||
          helperVersion ||
          ''
      ).trim()
      setHelperVersion(detectedVersion)
      if (
        detectedVersion &&
        requiredVersion &&
        detectedVersion !== requiredVersion
      ) {
        setHelperDetection('outdated')
        return
      }
      setHelperDetection('ready')
      if (helperProbeTimerRef.current) {
        window.clearTimeout(helperProbeTimerRef.current)
        helperProbeTimerRef.current = null
      }
    }
    window.addEventListener('message', handleHelperMessage)
    return () => {
      window.removeEventListener('message', handleHelperMessage)
      if (helperProbeTimerRef.current) {
        window.clearTimeout(helperProbeTimerRef.current)
        helperProbeTimerRef.current = null
      }
    }
  }, [
    captureID,
    captureStatus,
    captureStatus?.origin,
    captureStatus?.handoff_url,
    captureStatus?.helper_required_version,
    captureStatus?.helper_version,
    captureStatus?.platform,
    captureOrigin,
    captureRequiredHelperVersion,
    handoffURL,
    helperVersion,
    platform,
    t,
  ])

  async function prepareCaptureBridge(rawURL: string): Promise<string> {
    const targetURL = rawURL.trim()
    if (!targetURL) {
      throw new Error(t('Page bridge is not available yet.'))
    }
    if (bridgeScriptRef.current && bridgeURL === targetURL) {
      return bridgeScriptRef.current
    }
    setBridgeLoading(true)
    try {
      const response = await fetch(targetURL, {
        credentials: 'include',
        cache: 'no-store',
      })
      if (!response.ok) {
        throw new Error(t('Page bridge is not available yet.'))
      }
      const script = await response.text()
      if (!script.includes('nexustok-upstream-capture-bridge-result')) {
        throw new Error(t('Page bridge is not available yet.'))
      }
      bridgeScriptRef.current = script
      return script
    } finally {
      setBridgeLoading(false)
    }
  }

  async function handleRunCaptureBridge() {
    const targetURL = bridgeURL || captureStatus?.capture_bridge_url || ''
    if (!targetURL) {
      toast.error(t('Page bridge is not available yet.'))
      return
    }
    const targetWindow = activeCaptureWindowRef.current
    if (!targetWindow || targetWindow.closed) {
      const opened = openPlatformCaptureWindow(
        handoffURL || captureStatus?.handoff_url || '',
        targetWindow
      )
      if (!opened) {
        toast.error(t('Open the upstream capture page first.'))
        return
      }
      activeCaptureWindowRef.current = opened
      toast.info(t('Open the upstream page, then run the page bridge again.'))
      return
    }
    let script = ''
    try {
      script = await prepareCaptureBridge(targetURL)
      let targetOrigin = ''
      try {
        targetOrigin = window.location.origin
      } catch {
        targetOrigin = ''
      }
      if (!targetOrigin) {
        throw new Error(t('Page bridge is not available yet.'))
      }
      const bootstrap = createCaptureBridgeBootstrap(
        captureID || '',
        targetOrigin
      )
      setBridgeBookmarklet(bootstrap)
      try {
        await navigator.clipboard?.writeText(bootstrap)
        toast.success(t('Page bridge code copied'))
      } catch {
        toast.info(
          t('Copy the page bridge code and run it in the upstream page.')
        )
      }
      targetWindow.focus()
      setShowHandoffFallback(false)
    } catch {
      toast.error(
        script
          ? t('Copy the page bridge code and run it in the upstream page.')
          : t('Page bridge is not available yet.')
      )
    }
  }

  function handleStartCapture() {
    if (
      props.disabled ||
      captureMutation.isPending ||
      !(baseURL || '').trim()
    ) {
      return
    }
    setCaptureSessionExpired(false)
    setCaptureOrigin('')
    setCaptureRequiredHelperVersion('')
    setBridgeURL('')
    setBridgeBookmarklet('')
    bridgeCompletionRef.current = false
    try {
      pendingCaptureWindowRef.current = window.open('about:blank', '_blank')
      activeCaptureWindowRef.current = pendingCaptureWindowRef.current
    } catch {
      pendingCaptureWindowRef.current = null
      activeCaptureWindowRef.current = null
    }
    if (!pendingCaptureWindowRef.current) {
      setShowHandoffFallback(true)
      toast.info(
        t(
          'Browser blocked the upstream capture tab. Use the button below to continue.'
        )
      )
    }
    captureMutation.mutate()
  }

  function handleOpenHandoff() {
    const opened = openPlatformCaptureWindow(
      handoffURL || captureStatus?.handoff_url || '',
      activeCaptureWindowRef.current
    )
    if (!opened) {
      toast.error(t('Capture page is not ready yet'))
      return
    }
    activeCaptureWindowRef.current = opened
    setShowHandoffFallback(false)
  }

  useEffect(() => {
    if (ratioIsOverridden || previewRatio === undefined) {
      return
    }
    if (Math.abs(conversionRatio - previewRatio) <= RATIO_COMPARE_EPSILON) {
      return
    }
    form.setValue('platform_site_conversion_ratio', previewRatio, {
      shouldDirty: false,
      shouldValidate: false,
    })
  }, [conversionRatio, form, previewRatio, ratioIsOverridden])

  useEffect(() => {
    authFlowRef.current = authFlowID || ''
  }, [authFlowID])

  useEffect(() => {
    return () => {
      const flowID = authFlowRef.current
      if (flowID) {
        void cancelPlatformSiteAuthFlow(flowID)
      }
    }
  }, [])

  function clearAuthenticationFields() {
    const previousFlowID = authFlowRef.current
    if (previousFlowID) {
      void cancelPlatformSiteAuthFlow(previousFlowID)
    }
    authFlowRef.current = ''
    setAuthFlowStatus(undefined)
    form.setValue('platform_site_auth_flow_id', '')
    form.setValue('platform_site_password', '')
    form.setValue('platform_site_capture_id', '')
    setTwoFactorCode('')
    setHandoffURL('')
    setBridgeURL('')
    setBridgeBookmarklet('')
    bridgeScriptRef.current = ''
    setCaptureOrigin('')
    setCaptureRequiredHelperVersion('')
    setShowHandoffFallback(false)
    setCaptureSessionExpired(false)
    bridgeCompletionRef.current = false
    completedCaptureRef.current = ''
  }

  return (
    <fieldset
      disabled={props.disabled}
      className='space-y-4 disabled:opacity-60'
    >
      {props.syncStatus && (
        <div className='border-border/60 bg-background grid gap-1 rounded-md border p-3 text-sm'>
          <div className='flex flex-wrap items-center gap-2'>
            <span className='text-muted-foreground'>{t('Sync status')}</span>
            <span className='font-medium'>{props.syncStatus.sync_status}</span>
            {props.syncStatus.consecutive_failures > 0 && (
              <span className='text-destructive'>
                {t('Consecutive failures')}:{' '}
                {props.syncStatus.consecutive_failures}
              </span>
            )}
          </div>
          {props.syncStatus.last_sync_error && (
            <p className='text-destructive text-xs'>
              {props.syncStatus.last_sync_error}
            </p>
          )}
        </div>
      )}
      <div className='grid gap-4 sm:grid-cols-[minmax(0,2fr)_minmax(0,8fr)]'>
        <FormField
          control={form.control}
          name='platform_site_auth_type'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Authentication method')}</FormLabel>
              <Select
                value={field.value}
                onValueChange={(value) => {
                  field.onChange(value)
                  clearAuthenticationFields()
                }}
                items={[
                  { value: 'password', label: t('Username and password') },
                  { value: 'auto', label: t('Automatic configuration') },
                ]}
              >
                <FormControl>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                </FormControl>
                <SelectContent>
                  <SelectItem value='password'>
                    {t('Username and password')}
                  </SelectItem>
                  <SelectItem value='auto'>
                    {t('Automatic configuration')}
                  </SelectItem>
                </SelectContent>
              </Select>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name='base_url'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Site URL *')}</FormLabel>
              <FormControl>
                <Input
                  type='url'
                  placeholder='https://example.com'
                  {...field}
                />
              </FormControl>
              <FormDescription>
                {t(
                  'HTTP and HTTPS are supported, including private and local addresses.'
                )}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
      </div>

      {authType === 'password' && !authFlowID && (
        <div className='space-y-3'>
          <div className='grid gap-4 sm:grid-cols-2'>
            <FormField
              control={form.control}
              name='platform_site_username'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Username')}</FormLabel>
                  <FormControl>
                    <Input autoComplete='username' {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='platform_site_password'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Password')}</FormLabel>
                  <FormControl>
                    <Input
                      type='password'
                      autoComplete='new-password'
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
          <Button
            type='button'
            size='sm'
            onClick={() => passwordAuthMutation.mutate()}
            disabled={
              props.disabled ||
              passwordAuthMutation.isPending ||
              !(baseURL || '').trim() ||
              !username?.trim() ||
              !password
            }
          >
            {passwordAuthMutation.isPending ? (
              <Loader2 className='size-4 animate-spin' aria-hidden='true' />
            ) : (
              <KeyRound className='size-4' aria-hidden='true' />
            )}
            {t('Sign in to upstream site')}
          </Button>
        </div>
      )}

      {authType === 'password' && authFlowID && (
        <div className='border-border/60 bg-background grid gap-3 rounded-md border p-3'>
          <div className='flex items-center gap-2'>
            <ShieldCheck className='size-4' aria-hidden='true' />
            <span className='text-sm font-medium'>
              {authFlowStatus === 'two_factor_required'
                ? t('Two-factor verification required')
                : t('Upstream authentication verified')}
            </span>
          </div>
          {authFlowStatus === 'two_factor_required' && (
            <div className='grid gap-2 sm:max-w-xs'>
              <FormLabel htmlFor='platform-site-two-factor-code'>
                {t('Two-factor code')}
              </FormLabel>
              <Input
                id='platform-site-two-factor-code'
                inputMode='numeric'
                autoComplete='one-time-code'
                maxLength={6}
                value={twoFactorCode}
                onChange={(event) =>
                  setTwoFactorCode(
                    event.target.value.replaceAll(/\D/g, '').slice(0, 6)
                  )
                }
                placeholder={t('Enter the 6-digit code')}
              />
              <Button
                type='button'
                size='sm'
                onClick={() => verifyAuthFlowMutation.mutate()}
                disabled={
                  props.disabled ||
                  verifyAuthFlowMutation.isPending ||
                  twoFactorCode.length !== 6
                }
              >
                {verifyAuthFlowMutation.isPending && (
                  <Loader2 className='size-4 animate-spin' aria-hidden='true' />
                )}
                {t('Verify two-factor code')}
              </Button>
            </div>
          )}
        </div>
      )}

      {authType === 'auto' && (
        <div className='border-border/60 bg-background grid gap-3 rounded-md border p-3'>
          <div className='flex flex-wrap items-center gap-2'>
            <ShieldCheck className='size-4' aria-hidden='true' />
            <span className='text-sm font-medium'>
              {t('Browser login state capture')}
            </span>
            {captureStatus?.status === 'completed' && (
              <span className='text-muted-foreground text-xs'>
                {t('Captured')}
              </span>
            )}
          </div>
          {captureSessionExpired && (
            <p className='text-destructive text-xs' aria-live='polite'>
              {t('Capture session expired. Create a new session to continue.')}
            </p>
          )}
          {helperDetection !== 'idle' && (
            <p
              className={
                helperDetection === 'missing' || helperDetection === 'outdated'
                  ? 'text-destructive text-xs'
                  : 'text-muted-foreground text-xs'
              }
              aria-live='polite'
            >
              {helperDetection === 'probing' &&
                t('Detecting Capture Helper...')}
              {helperDetection === 'ready' &&
                t('Capture Helper is ready (version {{version}})', {
                  version: helperVersion || 'unknown',
                })}
              {helperDetection === 'missing' &&
                t('Capture Helper was not detected. Install it and try again.')}
              {helperDetection === 'outdated' &&
                t('Capture Helper is outdated. Update it before continuing.')}
            </p>
          )}
          <p className='text-muted-foreground text-xs'>
            {t(
              'Open the upstream site, complete login, then run the Capture Helper or page bridge. Tokens, cookies, and Admin Keys are never entered here.'
            )}
          </p>
          <div className='flex flex-wrap gap-2'>
            <Button
              type='button'
              size='sm'
              onClick={handleStartCapture}
              disabled={
                props.disabled ||
                captureMutation.isPending ||
                !(baseURL || '').trim()
              }
            >
              {captureMutation.isPending ? (
                <Loader2 className='size-4 animate-spin' aria-hidden='true' />
              ) : (
                <ShieldCheck className='size-4' aria-hidden='true' />
              )}
              {t(
                captureSessionExpired
                  ? 'Create a new capture session'
                  : 'Capture upstream login state'
              )}
            </Button>
            {showHandoffFallback &&
              (handoffURL || captureStatus?.handoff_url) && (
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={handleOpenHandoff}
                >
                  <ExternalLink className='size-4' aria-hidden='true' />
                  {t('Open upstream capture page')}
                </Button>
              )}
            {(captureStatus?.helper_install_url ||
              helperInstallURL ||
              captureStatus?.userscript_url) && (
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={() =>
                  window.open(
                    captureStatus?.helper_install_url ||
                      helperInstallURL ||
                      captureStatus?.userscript_url,
                    '_blank',
                    'noopener,noreferrer'
                  )
                }
              >
                <ExternalLink className='size-4' aria-hidden='true' />
                {t('Install Capture Helper')}
              </Button>
            )}
            {(bridgeURL || captureStatus?.capture_bridge_url) &&
              captureStatus?.status !== 'completed' && (
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={() => void handleRunCaptureBridge()}
                  disabled={bridgeLoading}
                >
                  {bridgeLoading ? (
                    <Loader2
                      className='size-4 animate-spin'
                      aria-hidden='true'
                    />
                  ) : (
                    <ExternalLink className='size-4' aria-hidden='true' />
                  )}
                  {t('Run page bridge')}
                </Button>
              )}
            {bridgeBookmarklet && captureStatus?.status !== 'completed' && (
              <CopyButton
                value={bridgeBookmarklet}
                variant='ghost'
                size='sm'
                tooltip={t('Copy page bridge code')}
                successTooltip={t('Page bridge code copied')}
              >
                {t('Copy page bridge code')}
              </CopyButton>
            )}
            {captureID && captureStatus?.status !== 'completed' && (
              <Button
                type='button'
                variant='ghost'
                size='sm'
                onClick={() => void captureStatusQuery.refetch()}
                disabled={captureStatusQuery.isFetching}
              >
                <RefreshCw className='size-4' aria-hidden='true' />
                {t('Refresh capture status')}
              </Button>
            )}
          </div>
          {captureStatus?.status === 'failed' && (
            <p className='text-destructive text-xs'>
              {captureStatus.message ||
                t('Upstream login state capture failed')}
            </p>
          )}
          {captureStatus?.summary && (
            <div className='text-muted-foreground grid gap-1 text-xs'>
              <span>
                {t('Authentication method')}:{' '}
                {formatPlatformCaptureAuthType(
                  captureStatus.summary.auth_type,
                  t
                )}
              </span>
              {captureStatus.summary.access_token_masked && (
                <span>
                  {t('Access token')}:{' '}
                  {captureStatus.summary.access_token_masked}
                </span>
              )}
              {captureStatus.summary.refresh_token_present && (
                <span>{t('Refresh token captured')}</span>
              )}
              {captureStatus.summary.admin_key_present && (
                <span>{t('Admin Key captured')}</span>
              )}
              {captureStatus.summary.cookie_present && (
                <span>{t('Cookie captured')}</span>
              )}
              {captureStatus.summary.token_expires_at && (
                <span>
                  {t('Token expires')}:{' '}
                  {new Date(
                    captureStatus.summary.token_expires_at * 1000
                  ).toLocaleString()}
                </span>
              )}
            </div>
          )}
        </div>
      )}

      <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
        <FormField
          control={form.control}
          name='platform_site_recharge_amount'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Recharge amount')}</FormLabel>
              <FormControl>
                <Input
                  type='number'
                  min='0'
                  step='0.001'
                  {...field}
                  onChange={(event) =>
                    field.onChange(
                      normalizePlatformNumber(event.target.valueAsNumber)
                    )
                  }
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='platform_site_credited_amount'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Credited amount')}</FormLabel>
              <FormControl>
                <Input
                  type='number'
                  min='0'
                  step='0.001'
                  {...field}
                  onChange={(event) =>
                    field.onChange(
                      normalizePlatformNumber(event.target.valueAsNumber)
                    )
                  }
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='platform_site_conversion_ratio'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Conversion ratio')}</FormLabel>
              <FormControl>
                <Input
                  type='number'
                  min='0'
                  step='0.001'
                  {...field}
                  value={formatPlatformRatioInput(field.value)}
                  onChange={(event) => {
                    field.onChange(
                      normalizePlatformRatio(
                        normalizePlatformNumber(event.target.valueAsNumber)
                      )
                    )
                    form.setValue(
                      'platform_site_conversion_ratio_override',
                      true,
                      { shouldDirty: true }
                    )
                  }}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='platform_site_conversion_ratio_override'
          render={({ field }) => (
            <FormItem className='flex min-h-9 items-center justify-between gap-3 self-end rounded-md border p-2'>
              <FormLabel className='cursor-pointer text-xs'>
                {t('Override conversion ratio')}
              </FormLabel>
              <FormControl>
                <Switch
                  checked={field.value}
                  onCheckedChange={(checked) => {
                    field.onChange(checked)
                    if (!checked) {
                      form.setValue(
                        'platform_site_conversion_ratio',
                        previewRatio ?? 1,
                        { shouldDirty: true, shouldValidate: false }
                      )
                    }
                  }}
                />
              </FormControl>
            </FormItem>
          )}
        />
      </div>
    </fieldset>
  )
}
