/**
 * OAuth 登录弹窗组件。
 *
 * 这个组件把原本独立 OAuth 页面中的认证流程收敛成一个可嵌入弹窗：
 * - 配额管理页面可以按供应商直接发起 OAuth 登录，用户无需离开当前页面。
 * - 登录成功后通过 onSuccess 通知调用方刷新认证文件列表，保证新账号能立即出现在配额区块里。
 * - 保留 callback URL 手动提交能力，兼容远程浏览器、容器环境和 provider 回调到本地端口失败的场景。
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { Modal } from '@/components/ui/Modal';
import { useNotificationStore, useThemeStore } from '@/stores';
import { oauthApi, type OAuthProvider } from '@/services/api/oauth';
import { copyToClipboard } from '@/utils/clipboard';
import styles from '@/pages/QuotaPage.module.scss';
import iconCodex from '@/assets/icons/codex.svg';
import iconClaude from '@/assets/icons/claude.svg';
import iconAntigravity from '@/assets/icons/antigravity.svg';
import iconGemini from '@/assets/icons/gemini.svg';
import iconKimiLight from '@/assets/icons/kimi-light.svg';
import iconKimiDark from '@/assets/icons/kimi-dark.svg';

type ProviderIcon = string | { light: string; dark: string };

interface ProviderDefinition {
  id: OAuthProvider;
  titleKey: string;
  hintKey: string;
  urlLabelKey: string;
  icon: ProviderIcon;
}

interface ProviderState {
  url?: string;
  state?: string;
  status?: 'idle' | 'waiting' | 'success' | 'error';
  error?: string;
  polling?: boolean;
  projectId?: string;
  projectIdError?: string;
  callbackUrl?: string;
  callbackSubmitting?: boolean;
  callbackStatus?: 'success' | 'error';
  callbackError?: string;
}

interface OAuthLoginModalProps {
  provider: OAuthProvider | null;
  open: boolean;
  onClose: () => void;
  onSuccess?: () => void | Promise<void>;
}

const PROVIDERS: ProviderDefinition[] = [
  { id: 'codex', titleKey: 'auth_login.codex_oauth_title', hintKey: 'auth_login.codex_oauth_hint', urlLabelKey: 'auth_login.codex_oauth_url_label', icon: iconCodex },
  { id: 'anthropic', titleKey: 'auth_login.anthropic_oauth_title', hintKey: 'auth_login.anthropic_oauth_hint', urlLabelKey: 'auth_login.anthropic_oauth_url_label', icon: iconClaude },
  { id: 'antigravity', titleKey: 'auth_login.antigravity_oauth_title', hintKey: 'auth_login.antigravity_oauth_hint', urlLabelKey: 'auth_login.antigravity_oauth_url_label', icon: iconAntigravity },
  { id: 'gemini-cli', titleKey: 'auth_login.gemini_cli_oauth_title', hintKey: 'auth_login.gemini_cli_oauth_hint', urlLabelKey: 'auth_login.gemini_cli_oauth_url_label', icon: iconGemini },
  { id: 'kimi', titleKey: 'auth_login.kimi_oauth_title', hintKey: 'auth_login.kimi_oauth_hint', urlLabelKey: 'auth_login.kimi_oauth_url_label', icon: { light: iconKimiLight, dark: iconKimiDark } },
];

const CALLBACK_SUPPORTED: OAuthProvider[] = [
  'codex',
  'anthropic',
  'antigravity',
  'gemini-cli',
];
const XAI_CALLBACK_URL = 'http://127.0.0.1:56121/callback';
const SUCCESS_RESET_DELAY_MS = 1200;
const POLLING_INTERVAL_MS = 3000;

const getProviderI18nPrefix = (provider: OAuthProvider) => provider.replace('-', '_');
const getAuthKey = (provider: OAuthProvider, suffix: string) =>
  `auth_login.${getProviderI18nPrefix(provider)}_${suffix}`;

const getIcon = (icon: ProviderIcon, theme: 'light' | 'dark') =>
  typeof icon === 'string' ? icon : icon[theme];

const isRecord = (value: unknown): value is Record<string, unknown> =>
  value !== null && typeof value === 'object';

const getErrorMessage = (error: unknown): string => {
  if (error instanceof Error) return error.message;
  if (isRecord(error) && typeof error.message === 'string') return error.message;
  return typeof error === 'string' ? error : '';
};

const getErrorStatus = (error: unknown): number | undefined => {
  if (!isRecord(error)) return undefined;
  return typeof error.status === 'number' ? error.status : undefined;
};

const isAbsoluteUrl = (value: string): boolean => {
  try {
    new URL(value);
    return true;
  } catch {
    return false;
  }
};

const readQueryLikeCallbackInput = (value: string) => {
  const trimmed = value.trim();
  if (!trimmed) return null;
  const queryStart = trimmed.indexOf('?');
  const hashStart = trimmed.indexOf('#');
  const rawParams =
    queryStart >= 0
      ? trimmed.slice(queryStart + 1)
      : hashStart >= 0
        ? trimmed.slice(hashStart + 1)
        : trimmed;

  if (!/(^|[&#?])(code|state|error)=/i.test(rawParams)) return null;
  return new URLSearchParams(rawParams.replace(/^[?#]/, ''));
};

const extractDisplayedXaiCode = (value: string): string => {
  const trimmed = value.trim();
  const codeMatch = trimmed.match(/\bcode\s*[:=]\s*([^\s&]+)/i);
  return (codeMatch?.[1] ?? trimmed).trim();
};

const buildXaiCallbackUrl = (input: string, state?: string): string | null => {
  const trimmed = input.trim();
  if (!trimmed) return null;
  if (isAbsoluteUrl(trimmed)) return trimmed;

  const params = readQueryLikeCallbackInput(trimmed);
  if (params) {
    const code = params.get('code')?.trim();
    const error = params.get('error')?.trim();
    const errorDescription = params.get('error_description')?.trim();
    const callbackState = params.get('state')?.trim() || state?.trim();
    if (!callbackState) return null;

    const callbackUrl = new URL(XAI_CALLBACK_URL);
    callbackUrl.searchParams.set('state', callbackState);
    if (code) callbackUrl.searchParams.set('code', code);
    if (error) callbackUrl.searchParams.set('error', error);
    if (errorDescription) callbackUrl.searchParams.set('error_description', errorDescription);
    return callbackUrl.toString();
  }

  const code = extractDisplayedXaiCode(trimmed);
  const callbackState = state?.trim();
  if (!code || !callbackState) return null;

  const callbackUrl = new URL(XAI_CALLBACK_URL);
  callbackUrl.searchParams.set('code', code);
  callbackUrl.searchParams.set('state', callbackState);
  return callbackUrl.toString();
};

const resolveCallbackUrl = (
  provider: OAuthProvider,
  input: string,
  state?: string
): string | null => {
  if (provider !== 'xai') return input.trim();
  return buildXaiCallbackUrl(input, state);
};

export const quotaOAuthProviders = PROVIDERS;

export function OAuthLoginModal({ provider, open, onClose, onSuccess }: OAuthLoginModalProps) {
  const { t } = useTranslation();
  const { showNotification } = useNotificationStore();
  const resolvedTheme = useThemeStore((state) => state.resolvedTheme);
  const [states, setStates] = useState<Record<string, ProviderState>>({});
  const pollingTimerRef = useRef<number | null>(null);
  const successResetTimerRef = useRef<number | null>(null);

  const definition = useMemo(
    () => PROVIDERS.find((item) => item.id === provider) ?? null,
    [provider]
  );
  const state = provider ? (states[provider] ?? {}) : {};

  const clearPollingTimer = useCallback(() => {
    if (pollingTimerRef.current !== null) {
      window.clearInterval(pollingTimerRef.current);
      pollingTimerRef.current = null;
    }
  }, []);

  const clearSuccessResetTimer = useCallback(() => {
    if (successResetTimerRef.current !== null) {
      window.clearTimeout(successResetTimerRef.current);
      successResetTimerRef.current = null;
    }
  }, []);

  const clearTimers = useCallback(() => {
    clearPollingTimer();
    clearSuccessResetTimer();
  }, [clearPollingTimer, clearSuccessResetTimer]);

  useEffect(() => {
    return () => {
      clearTimers();
    };
  }, [clearTimers]);

  const updateProviderState = useCallback((target: OAuthProvider, next: Partial<ProviderState>) => {
    setStates((prev) => ({
      ...prev,
      [target]: { ...(prev[target] ?? {}), ...next },
    }));
  }, []);

  const resetProviderAttempt = useCallback((target: OAuthProvider) => {
    clearTimers();
    setStates((prev) => {
      const current = prev[target] ?? {};
      const next: ProviderState = {};
      if (target === 'gemini-cli' && current.projectId !== undefined) {
        next.projectId = current.projectId;
      }
      return {
        ...prev,
        [target]: next,
      };
    });
  }, [clearTimers]);

  const completeProviderAuth = useCallback((target: OAuthProvider) => {
    clearPollingTimer();
    clearSuccessResetTimer();
    updateProviderState(target, {
      url: undefined,
      state: undefined,
      status: 'success',
      error: undefined,
      polling: false,
      callbackUrl: '',
      callbackSubmitting: false,
      callbackStatus: undefined,
      callbackError: undefined,
    });
    showNotification(t(getAuthKey(target, 'oauth_status_success')), 'success');
    void onSuccess?.();
    successResetTimerRef.current = window.setTimeout(() => {
      resetProviderAttempt(target);
      onClose();
    }, SUCCESS_RESET_DELAY_MS);
  }, [
    clearPollingTimer,
    clearSuccessResetTimer,
    onClose,
    onSuccess,
    resetProviderAttempt,
    showNotification,
    t,
    updateProviderState,
  ]);

  const startPolling = useCallback((target: OAuthProvider, authState: string) => {
    clearPollingTimer();
    const timer = window.setInterval(async () => {
      try {
        const res = await oauthApi.getAuthStatus(authState);
        if (res.status === 'ok') {
          completeProviderAuth(target);
        } else if (res.status === 'error') {
          updateProviderState(target, { status: 'error', error: res.error, polling: false });
          showNotification(
            `${t(getAuthKey(target, 'oauth_status_error'))} ${res.error || ''}`,
            'error'
          );
          clearPollingTimer();
        }
      } catch (err: unknown) {
        updateProviderState(target, {
          status: 'error',
          error: getErrorMessage(err),
          polling: false,
        });
        clearPollingTimer();
      }
    }, POLLING_INTERVAL_MS);
    pollingTimerRef.current = timer;
  }, [clearPollingTimer, completeProviderAuth, showNotification, t, updateProviderState]);

  const startAuth = useCallback(async () => {
    if (!provider) return;

    clearTimers();
    const geminiState = provider === 'gemini-cli' ? states[provider] : undefined;
    const rawProjectId = provider === 'gemini-cli' ? (geminiState?.projectId || '').trim() : '';
    const projectId = rawProjectId
      ? rawProjectId.toUpperCase() === 'ALL'
        ? 'ALL'
        : rawProjectId
      : undefined;

    if (provider === 'gemini-cli') {
      updateProviderState(provider, { projectIdError: undefined });
    }
    updateProviderState(provider, {
      url: undefined,
      state: undefined,
      status: 'waiting',
      polling: true,
      error: undefined,
      callbackStatus: undefined,
      callbackError: undefined,
      callbackUrl: '',
    });

    try {
      const res = await oauthApi.startAuth(
        provider,
        provider === 'gemini-cli' ? { projectId: projectId || undefined } : undefined
      );
      if (!res.state) {
        const message = t('auth_login.missing_state');
        updateProviderState(provider, {
          url: res.url,
          state: undefined,
          status: 'error',
          error: message,
          polling: false,
        });
        showNotification(message, 'error');
        return;
      }
      updateProviderState(provider, {
        url: res.url,
        state: res.state,
        status: 'waiting',
        polling: true,
      });
      startPolling(provider, res.state);
    } catch (err: unknown) {
      const message = getErrorMessage(err);
      updateProviderState(provider, { status: 'error', error: message, polling: false });
      showNotification(
        `${t(getAuthKey(provider, 'oauth_start_error'))}${message ? ` ${message}` : ''}`,
        'error'
      );
    }
  }, [clearTimers, provider, showNotification, startPolling, states, t, updateProviderState]);

  const copyLink = useCallback(async (url?: string) => {
    if (!url) return;
    const copied = await copyToClipboard(url);
    showNotification(
      t(copied ? 'notification.link_copied' : 'notification.copy_failed'),
      copied ? 'success' : 'error'
    );
  }, [showNotification, t]);

  const submitCallback = useCallback(async () => {
    if (!provider) return;

    const callbackInput = (states[provider]?.callbackUrl || '').trim();
    if (!callbackInput) {
      showNotification(
        t(provider === 'xai' ? 'auth_login.xai_callback_required' : 'auth_login.oauth_callback_required'),
        'warning'
      );
      return;
    }

    const redirectUrl = resolveCallbackUrl(provider, callbackInput, states[provider]?.state);
    if (!redirectUrl) {
      showNotification(
        t(provider === 'xai' ? 'auth_login.xai_callback_state_missing' : 'auth_login.missing_state'),
        'warning'
      );
      return;
    }

    updateProviderState(provider, {
      callbackSubmitting: true,
      callbackStatus: undefined,
      callbackError: undefined,
    });
    try {
      await oauthApi.submitCallback(provider, redirectUrl);
      updateProviderState(provider, { callbackSubmitting: false, callbackStatus: 'success' });
      showNotification(t('auth_login.oauth_callback_success'), 'success');
    } catch (err: unknown) {
      const status = getErrorStatus(err);
      const message = getErrorMessage(err);
      const errorMessage =
        status === 404
          ? t('auth_login.oauth_callback_upgrade_hint', {
              defaultValue: 'Please update CLI Proxy API or check the connection.',
            })
          : message || undefined;
      updateProviderState(provider, {
        callbackSubmitting: false,
        callbackStatus: 'error',
        callbackError: errorMessage,
      });
      const notificationMessage = errorMessage
        ? `${t('auth_login.oauth_callback_error')} ${errorMessage}`
        : t('auth_login.oauth_callback_error');
      showNotification(notificationMessage, 'error');
    }
  }, [provider, showNotification, states, t, updateProviderState]);

  const handleClose = useCallback(() => {
    if (provider) {
      resetProviderAttempt(provider);
    } else {
      clearTimers();
    }
    onClose();
  }, [clearTimers, onClose, provider, resetProviderAttempt]);

  if (!definition || !provider) {
    return <Modal open={open} title={t('quota_management.oauth_login_title')} onClose={onClose} />;
  }

  const canSubmitCallback = CALLBACK_SUPPORTED.includes(provider) && Boolean(state.url);
  const statusBadgeClassName = [
    'status-badge',
    state.status === 'success' ? 'success' : '',
    state.status === 'error' ? 'error' : '',
  ]
    .filter(Boolean)
    .join(' ');
  const loginButtonLabel =
    state.status === 'success'
      ? t('auth_login.login_another_account')
      : t(getAuthKey(provider, 'oauth_button'));

  return (
    <Modal
      open={open}
      title={
        <span className={styles.oauthModalTitle}>
          <img
            src={getIcon(definition.icon, resolvedTheme)}
            alt=""
            className={styles.oauthModalTitleIcon}
          />
          {t('quota_management.oauth_login_title_with_provider', {
            provider: t(definition.titleKey),
          })}
        </span>
      }
      onClose={handleClose}
      width={680}
    >
      <div className={styles.oauthModalContent}>
        <div className={styles.oauthModalHint}>{t(definition.hintKey)}</div>
        {provider === 'gemini-cli' && (
          <div className={styles.oauthModalField}>
            <Input
              label={t('auth_login.gemini_cli_project_id_label')}
              hint={t('auth_login.gemini_cli_project_id_hint')}
              value={state.projectId || ''}
              error={state.projectIdError}
              disabled={Boolean(state.polling)}
              onChange={(event) =>
                updateProviderState(provider, {
                  projectId: event.target.value,
                  projectIdError: undefined,
                })
              }
              placeholder={t('auth_login.gemini_cli_project_id_placeholder')}
            />
          </div>
        )}
        <div className={styles.oauthModalActions}>
          <Button onClick={() => void startAuth()} loading={state.polling}>
            {loginButtonLabel}
          </Button>
        </div>
        {state.url && (
          <div className={styles.oauthAuthUrlBox}>
            <div className={styles.oauthAuthUrlLabel}>{t(definition.urlLabelKey)}</div>
            <div className={styles.oauthAuthUrlValue}>{state.url}</div>
            <div className={styles.oauthModalActions}>
              <Button variant="secondary" size="sm" onClick={() => void copyLink(state.url)}>
                {t(getAuthKey(provider, 'copy_link'))}
              </Button>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => window.open(state.url, '_blank', 'noopener,noreferrer')}
              >
                {t(getAuthKey(provider, 'open_link'))}
              </Button>
            </div>
          </div>
        )}
        {canSubmitCallback && (
          <div className={styles.oauthCallbackSection}>
            <Input
              label={t(
                provider === 'xai' ? 'auth_login.xai_callback_label' : 'auth_login.oauth_callback_label'
              )}
              hint={t(
                provider === 'xai' ? 'auth_login.xai_callback_hint' : 'auth_login.oauth_callback_hint'
              )}
              value={state.callbackUrl || ''}
              onChange={(event) =>
                updateProviderState(provider, {
                  callbackUrl: event.target.value,
                  callbackStatus: undefined,
                  callbackError: undefined,
                })
              }
              placeholder={t(
                provider === 'xai'
                  ? 'auth_login.xai_callback_placeholder'
                  : 'auth_login.oauth_callback_placeholder'
              )}
            />
            <div className={styles.oauthModalActions}>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => void submitCallback()}
                loading={state.callbackSubmitting}
              >
                {t('auth_login.oauth_callback_button')}
              </Button>
            </div>
            {state.callbackStatus === 'success' && state.status === 'waiting' && (
              <div className="status-badge success">
                {t('auth_login.oauth_callback_status_success')}
              </div>
            )}
            {state.callbackStatus === 'error' && (
              <div className="status-badge error">
                {t('auth_login.oauth_callback_status_error')} {state.callbackError || ''}
              </div>
            )}
          </div>
        )}
        {state.status && state.status !== 'idle' && (
          <div className={statusBadgeClassName}>
            {state.status === 'success'
              ? t(getAuthKey(provider, 'oauth_status_success'))
              : state.status === 'error'
                ? `${t(getAuthKey(provider, 'oauth_status_error'))} ${state.error || ''}`
                : t(getAuthKey(provider, 'oauth_status_waiting'))}
          </div>
        )}
      </div>
    </Modal>
  );
}
