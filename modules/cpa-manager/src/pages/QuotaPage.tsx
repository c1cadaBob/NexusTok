/**
 * 配额管理页面，负责统一加载认证文件、配置文件，并协调各供应商配额区块。
 */

import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { useHeaderRefresh } from '@/hooks/useHeaderRefresh';
import { useAuthStore } from '@/stores';
import { authFilesApi, configFileApi } from '@/services/api';
import { Card } from '@/components/ui/Card';
import { Input } from '@/components/ui/Input';
import { Select } from '@/components/ui/Select';
import {
  IconChartLine,
  IconCrosshair,
  IconFileText,
  IconInfo,
  IconKey,
  IconSettings,
  IconModelCluster,
  IconScrollText,
  IconSearch,
  IconShield,
  IconSlidersHorizontal,
} from '@/components/ui/icons';
import {
  QuotaProviderNav,
  QuotaSection,
  ANTIGRAVITY_CONFIG,
  CLAUDE_CONFIG,
  CODEX_CONFIG,
  GEMINI_CLI_CONFIG,
  KIMI_CONFIG
} from '@/components/quota';
import {
  OAuthLoginModal,
  getOAuthProviderIcon,
  quotaOAuthProviders
} from '@/components/oauth/OAuthLoginModal';
import { VertexImportModal } from '@/components/oauth/VertexImportModal';
import { useThemeStore } from '@/stores';
import type { QuotaSortMode, QuotaType } from '@/components/quota/quotaConfigs';
import type { OAuthProvider } from '@/services/api/oauth';
import type { AuthFileItem } from '@/types';
import {
  getAuthFileIcon,
  getTypeLabel,
  supportsOAuthModelAlias,
} from '@/features/authFiles/constants';
import styles from './QuotaPage.module.scss';
import iconVertex from '@/assets/icons/vertex.svg';

const QUOTA_OAUTH_PROVIDER_MAP: Record<QuotaType, OAuthProvider> = {
  codex: 'codex',
  claude: 'anthropic',
  antigravity: 'antigravity',
  'gemini-cli': 'gemini-cli',
  kimi: 'kimi',
};

type OAuthSettingsShortcut = {
  path: string;
  titleKey: string;
  descKey: string;
  icon: ReactNode;
};

type OAuthRuleShortcutKind = 'excluded' | 'alias';

type AccountTypeShortcut = {
  provider: string;
  loginProvider?: OAuthProvider;
  importKind?: 'vertex';
};

type UpstreamConfigShortcut = {
  provider: string;
  path: string;
  titleKey: string;
  descKey: string;
  actionKey: string;
};

type OperationsShortcut = {
  path: string;
  titleKey: string;
  descKey: string;
  icon: ReactNode;
};

// 配额页是账号池日常运维的起点。这里保留的快捷入口必须全部指向
// CPAMC 已经存在且可以直接访问的真实页面，避免把“账号类型”“模型规则”
// 和真正可发起 OAuth callback/device flow 的登录入口混在一起。
const OAUTH_SETTINGS_SHORTCUTS: OAuthSettingsShortcut[] = [
  {
    path: '/auth-files',
    titleKey: 'quota_management.settings_auth_files',
    descKey: 'quota_management.settings_auth_files_desc',
    icon: <IconFileText size={20} />,
  },
  {
    path: '/account-groups',
    titleKey: 'quota_management.settings_account_groups',
    descKey: 'quota_management.settings_account_groups_desc',
    icon: <IconModelCluster size={20} />,
  },
  {
    path: '/ai-providers',
    titleKey: 'quota_management.settings_ai_providers',
    descKey: 'quota_management.settings_ai_providers_desc',
    icon: <IconSlidersHorizontal size={20} />,
  },
  {
    path: '/config',
    titleKey: 'quota_management.settings_runtime_config',
    descKey: 'quota_management.settings_runtime_config_desc',
    icon: <IconSettings size={20} />,
  },
  {
    path: '/auth-files/oauth-excluded',
    titleKey: 'quota_management.settings_oauth_excluded',
    descKey: 'quota_management.settings_oauth_excluded_desc',
    icon: <IconShield size={20} />,
  },
  {
    path: '/auth-files/oauth-model-alias',
    titleKey: 'quota_management.settings_oauth_model_alias',
    descKey: 'quota_management.settings_oauth_model_alias_desc',
    icon: <IconScrollText size={20} />,
  },
];

const OPERATIONS_SHORTCUTS: OperationsShortcut[] = [
  {
    path: '/monitoring',
    titleKey: 'quota_management.ops_monitoring_title',
    descKey: 'quota_management.ops_monitoring_desc',
    icon: <IconChartLine size={20} />,
  },
  {
    path: '/monitoring/codex-inspection',
    titleKey: 'quota_management.ops_codex_inspection_title',
    descKey: 'quota_management.ops_codex_inspection_desc',
    icon: <IconCrosshair size={20} />,
  },
  {
    path: '/logs',
    titleKey: 'quota_management.ops_logs_title',
    descKey: 'quota_management.ops_logs_desc',
    icon: <IconScrollText size={20} />,
  },
  {
    path: '/system',
    titleKey: 'quota_management.ops_system_title',
    descKey: 'quota_management.ops_system_desc',
    icon: <IconInfo size={20} />,
  },
];

const OAUTH_RULE_PROVIDERS = [
  'codex',
  'claude',
  'antigravity',
  'gemini-cli',
  'gemini',
  'kimi',
  'xai',
  'vertex',
  'aistudio',
  'qwen',
  'iflow',
];

const ACCOUNT_TYPE_SHORTCUTS: AccountTypeShortcut[] = [
  { provider: 'codex', loginProvider: 'codex' },
  { provider: 'claude', loginProvider: 'anthropic' },
  { provider: 'antigravity', loginProvider: 'antigravity' },
  { provider: 'gemini-cli', loginProvider: 'gemini-cli' },
  { provider: 'gemini' },
  { provider: 'kimi', loginProvider: 'kimi' },
  { provider: 'xai', loginProvider: 'xai' },
  { provider: 'vertex', importKind: 'vertex' },
  { provider: 'aistudio' },
  { provider: 'qwen' },
  { provider: 'iflow' },
];

const UPSTREAM_CONFIG_SHORTCUTS: UpstreamConfigShortcut[] = [
  {
    provider: 'gemini',
    path: '/ai-providers/gemini/new',
    titleKey: 'quota_management.upstream_gemini_title',
    descKey: 'quota_management.upstream_gemini_desc',
    actionKey: 'quota_management.upstream_add_action',
  },
  {
    provider: 'codex',
    path: '/ai-providers/codex/new',
    titleKey: 'quota_management.upstream_codex_title',
    descKey: 'quota_management.upstream_codex_desc',
    actionKey: 'quota_management.upstream_add_action',
  },
  {
    provider: 'claude',
    path: '/ai-providers/claude/new',
    titleKey: 'quota_management.upstream_claude_title',
    descKey: 'quota_management.upstream_claude_desc',
    actionKey: 'quota_management.upstream_add_action',
  },
  {
    provider: 'vertex',
    path: '/ai-providers/vertex/new',
    titleKey: 'quota_management.upstream_vertex_title',
    descKey: 'quota_management.upstream_vertex_desc',
    actionKey: 'quota_management.upstream_add_action',
  },
  {
    provider: 'openai',
    path: '/ai-providers/openai/new',
    titleKey: 'quota_management.upstream_openai_title',
    descKey: 'quota_management.upstream_openai_desc',
    actionKey: 'quota_management.upstream_add_action',
  },
  {
    provider: 'ampcode',
    path: '/ai-providers/ampcode',
    titleKey: 'quota_management.upstream_ampcode_title',
    descKey: 'quota_management.upstream_ampcode_desc',
    actionKey: 'quota_management.upstream_configure_action',
  },
];

const buildOAuthRulePath = (provider: string, kind: OAuthRuleShortcutKind) => {
  const basePath =
    kind === 'excluded' ? '/auth-files/oauth-excluded' : '/auth-files/oauth-model-alias';
  return `${basePath}?provider=${encodeURIComponent(provider)}`;
};

const buildAuthFilesTypePath = (provider: string) =>
  `/auth-files?type=${encodeURIComponent(provider)}`;

export function QuotaPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const connectionStatus = useAuthStore((state) => state.connectionStatus);
  const resolvedTheme = useThemeStore((state) => state.resolvedTheme);

  const [files, setFiles] = useState<AuthFileItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [searchQuery, setSearchQuery] = useState('');
  const [sortMode, setSortMode] = useState<QuotaSortMode>('default');
  const [loginProvider, setLoginProvider] = useState<OAuthProvider | null>(null);
  const [vertexImportOpen, setVertexImportOpen] = useState(false);

  const disableControls = connectionStatus !== 'connected';
  const sortOptions = useMemo(
    () => [
      { value: 'default', label: t('quota_management.sort_default') },
      { value: 'name-asc', label: t('quota_management.sort_name_asc') },
      { value: 'plan-desc', label: t('quota_management.sort_plan_desc') },
      { value: 'plan-asc', label: t('quota_management.sort_plan_asc') }
    ],
    [t]
  );

  const loadConfig = useCallback(async () => {
    try {
      await configFileApi.fetchConfigYaml();
    } catch (err: unknown) {
      const errorMessage = err instanceof Error ? err.message : t('notification.refresh_failed');
      setError((prev) => prev || errorMessage);
    }
  }, [t]);

  const loadFiles = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const data = await authFilesApi.list();
      setFiles(data?.files || []);
    } catch (err: unknown) {
      const errorMessage = err instanceof Error ? err.message : t('notification.refresh_failed');
      setError(errorMessage);
    } finally {
      setLoading(false);
    }
  }, [t]);

  const handleHeaderRefresh = useCallback(async () => {
    await Promise.all([loadConfig(), loadFiles()]);
  }, [loadConfig, loadFiles]);

  useHeaderRefresh(handleHeaderRefresh);

  useEffect(() => {
    loadFiles();
    loadConfig();
  }, [loadFiles, loadConfig]);

  const getLoginLabel = useCallback(
    (type: QuotaType) => {
      const provider = QUOTA_OAUTH_PROVIDER_MAP[type];
      const definition = quotaOAuthProviders.find((item) => item.id === provider);
      return t('quota_management.login_provider', {
        provider: definition ? t(definition.titleKey) : provider,
      });
    },
    [t]
  );

  const openLoginModal = useCallback((type: QuotaType) => {
    setLoginProvider(QUOTA_OAUTH_PROVIDER_MAP[type]);
  }, []);

  const openOAuthRule = useCallback(
    (provider: string, kind: OAuthRuleShortcutKind) => {
      navigate(buildOAuthRulePath(provider, kind), { state: { fromAuthFiles: true } });
    },
    [navigate]
  );

  const openAccountType = useCallback(
    (provider: string) => {
      navigate(buildAuthFilesTypePath(provider));
    },
    [navigate]
  );

  const openAccountTypeLogin = useCallback((shortcut: AccountTypeShortcut) => {
    if (shortcut.importKind === 'vertex') {
      setVertexImportOpen(true);
      return;
    }
    if (shortcut.loginProvider) {
      setLoginProvider(shortcut.loginProvider);
    }
  }, []);

  const openUpstreamConfig = useCallback(
    (shortcut: UpstreamConfigShortcut) => {
      navigate(shortcut.path, { state: { fromAiProviders: true } });
    },
    [navigate]
  );

  return (
    <div className={styles.container}>
      <div className={styles.pageHeader}>
        <h1 className={styles.pageTitle}>{t('quota_management.title')}</h1>
        <p className={styles.description}>{t('quota_management.description')}</p>
      </div>

      {error && <div className={styles.errorBox}>{error}</div>}

      <Card
        className={styles.oauthLoginCard}
        title={t('quota_management.oauth_shortcuts_title')}
      >
        <div className={styles.oauthShortcutGrid}>
          {quotaOAuthProviders.map((provider) => (
            <button
              key={provider.id}
              type="button"
              className={styles.oauthShortcutButton}
              onClick={() => setLoginProvider(provider.id)}
              disabled={disableControls}
            >
              <img
                src={getOAuthProviderIcon(provider.icon, resolvedTheme)}
                alt=""
                className={styles.oauthShortcutIcon}
              />
              <span className={styles.oauthShortcutText}>{t(provider.titleKey)}</span>
            </button>
          ))}
          <button
            type="button"
            className={styles.oauthShortcutButton}
            onClick={() => setVertexImportOpen(true)}
            disabled={disableControls}
          >
            <img src={iconVertex} alt="" className={styles.oauthShortcutIcon} />
            <span className={styles.oauthShortcutText}>{t('vertex_import.title')}</span>
          </button>
        </div>
        <div className={styles.oauthShortcutHint}>
          {t('quota_management.oauth_shortcuts_desc')}
        </div>
      </Card>

      <Card
        className={styles.oauthSettingsCard}
        title={t('quota_management.oauth_settings_title')}
      >
        <div className={styles.oauthSettingsGrid}>
          {OAUTH_SETTINGS_SHORTCUTS.map((item) => (
            <button
              key={item.path}
              type="button"
              className={styles.oauthSettingsButton}
              onClick={() => navigate(item.path)}
            >
              <span className={styles.oauthSettingsIcon}>{item.icon}</span>
              <span className={styles.oauthSettingsContent}>
                <span className={styles.oauthSettingsTitle}>{t(item.titleKey)}</span>
                <span className={styles.oauthSettingsDesc}>{t(item.descKey)}</span>
              </span>
            </button>
          ))}
        </div>
        <div className={styles.oauthShortcutHint}>
          {t('quota_management.oauth_settings_desc')}
        </div>
      </Card>

      <Card
        className={styles.oauthRulesCard}
        title={t('quota_management.oauth_rule_shortcuts_title')}
      >
        <div className={styles.oauthRulesGrid}>
          {OAUTH_RULE_PROVIDERS.map((provider) => {
            const icon = getAuthFileIcon(provider, resolvedTheme);
            return (
              <div key={provider} className={styles.oauthRuleProvider}>
                <div className={styles.oauthRuleProviderHeader}>
                  {icon ? (
                    <img src={icon} alt="" className={styles.oauthRuleProviderIcon} />
                  ) : (
                    <span className={styles.oauthRuleProviderIconFallback}>
                      {getTypeLabel(t, provider).slice(0, 1)}
                    </span>
                  )}
                  <span className={styles.oauthRuleProviderName}>
                    {getTypeLabel(t, provider)}
                  </span>
                </div>
                <div className={styles.oauthRuleActions}>
                  <button
                    type="button"
                    className={styles.oauthRuleActionButton}
                    onClick={() => openOAuthRule(provider, 'excluded')}
                  >
                    {t('quota_management.oauth_rule_excluded_action')}
                  </button>
                  {supportsOAuthModelAlias(provider) ? (
                    <button
                      type="button"
                      className={styles.oauthRuleActionButton}
                      onClick={() => openOAuthRule(provider, 'alias')}
                    >
                      {t('quota_management.oauth_rule_alias_action')}
                    </button>
                  ) : (
                    <span className={styles.oauthRuleManualOnly}>
                      {t('quota_management.oauth_rule_manual_only')}
                    </span>
                  )}
                </div>
              </div>
            );
          })}
        </div>
        <div className={styles.oauthShortcutHint}>
          {t('quota_management.oauth_rule_shortcuts_desc')}
        </div>
      </Card>

      <Card
        className={styles.upstreamConfigCard}
        title={t('quota_management.upstream_config_shortcuts_title')}
      >
        <div className={styles.upstreamConfigGrid}>
          {UPSTREAM_CONFIG_SHORTCUTS.map((shortcut) => {
            const icon = getAuthFileIcon(shortcut.provider, resolvedTheme);

            return (
              <button
                key={shortcut.provider}
                type="button"
                className={styles.upstreamConfigButton}
                onClick={() => openUpstreamConfig(shortcut)}
              >
                {icon ? (
                  <img src={icon} alt="" className={styles.upstreamConfigIcon} />
                ) : (
                  <span className={styles.upstreamConfigIconFallback}>
                    <IconKey size={18} />
                  </span>
                )}
                <span className={styles.upstreamConfigContent}>
                  <span className={styles.upstreamConfigTitle}>{t(shortcut.titleKey)}</span>
                  <span className={styles.upstreamConfigDesc}>{t(shortcut.descKey)}</span>
                  <span className={styles.upstreamConfigAction}>{t(shortcut.actionKey)}</span>
                </span>
              </button>
            );
          })}
        </div>
        <div className={styles.oauthShortcutHint}>
          {t('quota_management.upstream_config_shortcuts_desc')}
        </div>
      </Card>

      <Card
        className={styles.accountTypeCard}
        title={t('quota_management.account_type_shortcuts_title')}
      >
        <div className={styles.accountTypeGrid}>
          {ACCOUNT_TYPE_SHORTCUTS.map((shortcut) => {
            const icon = getAuthFileIcon(shortcut.provider, resolvedTheme);
            const hasLoginEntry = Boolean(shortcut.loginProvider || shortcut.importKind);
            const loginLabel =
              shortcut.importKind === 'vertex'
                ? t('quota_management.account_type_import_action')
                : t('quota_management.account_type_login_action');

            return (
              <div key={shortcut.provider} className={styles.accountTypeProvider}>
                <div className={styles.accountTypeProviderHeader}>
                  {icon ? (
                    <img src={icon} alt="" className={styles.accountTypeProviderIcon} />
                  ) : (
                    <span className={styles.accountTypeProviderIconFallback}>
                      {getTypeLabel(t, shortcut.provider).slice(0, 1)}
                    </span>
                  )}
                  <span className={styles.accountTypeProviderName}>
                    {getTypeLabel(t, shortcut.provider)}
                  </span>
                </div>
                <div className={styles.accountTypeActions}>
                  <button
                    type="button"
                    className={styles.accountTypePrimaryAction}
                    onClick={() => openAccountType(shortcut.provider)}
                  >
                    {t('quota_management.account_type_view_accounts_action')}
                  </button>
                  {hasLoginEntry && (
                    <button
                      type="button"
                      className={styles.accountTypeActionButton}
                      onClick={() => openAccountTypeLogin(shortcut)}
                      disabled={disableControls}
                    >
                      {loginLabel}
                    </button>
                  )}
                  <button
                    type="button"
                    className={styles.accountTypeActionButton}
                    onClick={() => openOAuthRule(shortcut.provider, 'excluded')}
                  >
                    {t('quota_management.oauth_rule_excluded_action')}
                  </button>
                  {supportsOAuthModelAlias(shortcut.provider) && (
                    <button
                      type="button"
                      className={styles.accountTypeActionButton}
                      onClick={() => openOAuthRule(shortcut.provider, 'alias')}
                    >
                      {t('quota_management.oauth_rule_alias_action')}
                    </button>
                  )}
                </div>
              </div>
            );
          })}
        </div>
        <div className={styles.oauthShortcutHint}>
          {t('quota_management.account_type_shortcuts_desc')}
        </div>
      </Card>

      <Card
        className={styles.opsShortcutCard}
        title={t('quota_management.ops_shortcuts_title')}
      >
        <div className={styles.opsShortcutGrid}>
          {OPERATIONS_SHORTCUTS.map((item) => (
            <button
              key={item.path}
              type="button"
              className={styles.opsShortcutButton}
              onClick={() => navigate(item.path)}
            >
              <span className={styles.opsShortcutIcon}>{item.icon}</span>
              <span className={styles.opsShortcutContent}>
                <span className={styles.opsShortcutTitle}>{t(item.titleKey)}</span>
                <span className={styles.opsShortcutDesc}>{t(item.descKey)}</span>
              </span>
            </button>
          ))}
        </div>
        <div className={styles.oauthShortcutHint}>
          {t('quota_management.ops_shortcuts_desc')}
        </div>
      </Card>

      <div className={styles.toolbar}>
        <div className={styles.toolbarField}>
          <Input
            label={t('quota_management.search_label')}
            value={searchQuery}
            onChange={(event) => setSearchQuery(event.target.value)}
            placeholder={t('quota_management.search_placeholder')}
            rightElement={<IconSearch size={16} />}
            aria-label={t('quota_management.search_label')}
          />
        </div>
        <div className={`${styles.toolbarField} ${styles.sortField}`}>
          <label htmlFor="quota-sort-mode" className={styles.toolbarLabel}>
            {t('quota_management.sort_label')}
          </label>
          <Select
            id="quota-sort-mode"
            value={sortMode}
            options={sortOptions}
            onChange={(value) => setSortMode(value as QuotaSortMode)}
            ariaLabel={t('quota_management.sort_label')}
            fullWidth
          />
        </div>
      </div>

      <QuotaSection
        config={CODEX_CONFIG}
        files={files}
        loading={loading}
        disabled={disableControls}
        searchQuery={searchQuery}
        sortMode={sortMode}
        onLogin={() => openLoginModal(CODEX_CONFIG.type)}
        loginLabel={getLoginLabel(CODEX_CONFIG.type)}
      />
      <QuotaSection
        config={CLAUDE_CONFIG}
        files={files}
        loading={loading}
        disabled={disableControls}
        searchQuery={searchQuery}
        sortMode={sortMode}
        onLogin={() => openLoginModal(CLAUDE_CONFIG.type)}
        loginLabel={getLoginLabel(CLAUDE_CONFIG.type)}
      />
      <QuotaSection
        config={ANTIGRAVITY_CONFIG}
        files={files}
        loading={loading}
        disabled={disableControls}
        searchQuery={searchQuery}
        sortMode={sortMode}
        onLogin={() => openLoginModal(ANTIGRAVITY_CONFIG.type)}
        loginLabel={getLoginLabel(ANTIGRAVITY_CONFIG.type)}
      />
      <QuotaSection
        config={GEMINI_CLI_CONFIG}
        files={files}
        loading={loading}
        disabled={disableControls}
        searchQuery={searchQuery}
        sortMode={sortMode}
        onLogin={() => openLoginModal(GEMINI_CLI_CONFIG.type)}
        loginLabel={getLoginLabel(GEMINI_CLI_CONFIG.type)}
      />
      <QuotaSection
        config={KIMI_CONFIG}
        files={files}
        loading={loading}
        disabled={disableControls}
        searchQuery={searchQuery}
        sortMode={sortMode}
        onLogin={() => openLoginModal(KIMI_CONFIG.type)}
        loginLabel={getLoginLabel(KIMI_CONFIG.type)}
      />
      <QuotaProviderNav />
      <OAuthLoginModal
        provider={loginProvider}
        open={Boolean(loginProvider)}
        onClose={() => setLoginProvider(null)}
        onSuccess={loadFiles}
      />
      <VertexImportModal
        open={vertexImportOpen}
        onClose={() => setVertexImportOpen(false)}
        onSuccess={loadFiles}
      />
    </div>
  );
}
