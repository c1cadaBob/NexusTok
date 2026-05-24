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
  IconInfo,
  IconScrollText,
  IconSearch,
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
import { OAuthLoginModal } from '@/components/oauth/OAuthLoginModal';
import type { QuotaSortMode, QuotaType } from '@/components/quota/quotaConfigs';
import type { OAuthProvider } from '@/services/api/oauth';
import type { AuthFileItem } from '@/types';
import styles from './QuotaPage.module.scss';

const QUOTA_OAUTH_PROVIDER_MAP: Record<QuotaType, OAuthProvider> = {
  codex: 'codex',
  claude: 'anthropic',
  antigravity: 'antigravity',
  'gemini-cli': 'gemini-cli',
  kimi: 'kimi',
};

const QUOTA_LOGIN_TITLE_KEY_MAP: Record<QuotaType, string> = {
  codex: 'auth_login.codex_oauth_title',
  claude: 'auth_login.anthropic_oauth_title',
  antigravity: 'auth_login.antigravity_oauth_title',
  'gemini-cli': 'auth_login.gemini_cli_oauth_title',
  kimi: 'auth_login.kimi_oauth_title',
};

type OperationsShortcut = {
  path: string;
  titleKey: string;
  descKey: string;
  icon: ReactNode;
};

// 配额页只保留账号池日常维护所需的运行诊断入口。
// OAuth 登录入口下沉到对应供应商额度区块的右上角，和额度刷新、分页切换放在同一操作区；
// 这样管理员在查看某个供应商额度时可以就地登录补充账号，不需要先回到顶部寻找入口。
// 供应商 API Key、Base URL、模型范围等上游配置已经由 NexusTok 主项目
// 的渠道管理、模型管理和模型定价分组承接，CPAMC 内不再重复展示这些入口，
// 避免管理员在两个系统中维护同类配置时产生来源不一致的问题。
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

export function QuotaPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const connectionStatus = useAuthStore((state) => state.connectionStatus);

  const [files, setFiles] = useState<AuthFileItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [searchQuery, setSearchQuery] = useState('');
  const [sortMode, setSortMode] = useState<QuotaSortMode>('default');
  const [loginProvider, setLoginProvider] = useState<OAuthProvider | null>(null);

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
      return t('quota_management.login_provider', {
        provider: t(QUOTA_LOGIN_TITLE_KEY_MAP[type]),
      });
    },
    [t]
  );

  const openLoginModal = useCallback((type: QuotaType) => {
    setLoginProvider(QUOTA_OAUTH_PROVIDER_MAP[type]);
  }, []);

  return (
    <div className={styles.container}>
      <div className={styles.pageHeader}>
        <h1 className={styles.pageTitle}>{t('quota_management.title')}</h1>
        <p className={styles.description}>{t('quota_management.description')}</p>
      </div>

      {error && <div className={styles.errorBox}>{error}</div>}

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
        <div className={styles.shortcutHint}>
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
    </div>
  );
}
