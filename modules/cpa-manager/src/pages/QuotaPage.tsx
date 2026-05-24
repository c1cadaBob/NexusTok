/**
 * 配额管理页面，负责统一加载认证文件、配置文件，并协调各供应商配额区块。
 */

import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { useHeaderRefresh } from '@/hooks/useHeaderRefresh';
import { useAuthStore } from '@/stores';
import { authFilesApi, configFileApi } from '@/services/api';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { EmptyState } from '@/components/ui/EmptyState';
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
import { VertexImportModal } from '@/components/oauth/VertexImportModal';
import type { QuotaSortMode, QuotaType } from '@/components/quota/quotaConfigs';
import type { OAuthProvider } from '@/services/api/oauth';
import type { AuthFileItem } from '@/types';
import styles from './QuotaPage.module.scss';

const QUOTA_OAUTH_PROVIDER_MAP: Partial<Record<QuotaType, OAuthProvider>> = {
  codex: 'codex',
  claude: 'anthropic',
  antigravity: 'antigravity',
  'gemini-cli': 'gemini-cli',
  kimi: 'kimi',
  xai: 'xai',
};

const QUOTA_LOGIN_TITLE_KEY_MAP: Partial<Record<QuotaType, string>> = {
  codex: 'auth_login.codex_oauth_title',
  claude: 'auth_login.anthropic_oauth_title',
  antigravity: 'auth_login.antigravity_oauth_title',
  'gemini-cli': 'auth_login.gemini_cli_oauth_title',
  kimi: 'auth_login.kimi_oauth_title',
  xai: 'auth_login.xai_oauth_title',
};

type OperationsShortcut = {
  path: string;
  titleKey: string;
  descKey: string;
  icon: ReactNode;
};

type SupplementalProviderId = 'xai' | 'vertex' | 'kiro';

type SupplementalProviderAction = 'oauth' | 'vertex-import' | 'reserved';

type SupplementalProviderDefinition = {
  id: SupplementalProviderId;
  titleKey: string;
  emptyTitleKey: string;
  emptyDescKey: string;
  hintKey: string;
  action: SupplementalProviderAction;
  actionLabelKey: string;
  actionProviderTitleKey?: string;
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

// xAI、Vertex、Kiro 目前还没有接入和 Codex/Gemini CLI 相同的自动额度查询器，
// 但它们仍然属于账号池官方账号能力的一部分。这里把它们作为“补充供应商区块”
// 下沉到配额页下方，统一承担登录、导入、预留和凭证可见性职责：
// - xAI 复用已有 OAuth callback 流程，可以直接发起登录；
// - Vertex 不是 OAuth 流程，而是服务账号 JSON 导入，因此使用独立导入弹窗；
// - Kiro 暂未接入 provider runtime，先固定展示预留区块，方便后续补齐能力时不再调整页面结构。
const SUPPLEMENTAL_PROVIDERS: SupplementalProviderDefinition[] = [
  {
    id: 'xai',
    titleKey: 'xai_quota.title',
    emptyTitleKey: 'xai_quota.empty_title',
    emptyDescKey: 'xai_quota.empty_desc',
    hintKey: 'xai_quota.hint',
    action: 'oauth',
    actionLabelKey: 'quota_management.login_provider',
    actionProviderTitleKey: 'auth_login.xai_oauth_title',
  },
  {
    id: 'vertex',
    titleKey: 'vertex_quota.title',
    emptyTitleKey: 'vertex_quota.empty_title',
    emptyDescKey: 'vertex_quota.empty_desc',
    hintKey: 'vertex_quota.hint',
    action: 'vertex-import',
    actionLabelKey: 'vertex_import.title',
  },
  {
    id: 'kiro',
    titleKey: 'kiro_quota.title',
    emptyTitleKey: 'kiro_quota.empty_title',
    emptyDescKey: 'kiro_quota.empty_desc',
    hintKey: 'kiro_quota.hint',
    action: 'reserved',
    actionLabelKey: 'kiro_quota.reserved_button',
  },
];

const normalizeProviderKey = (value: unknown): string => {
  const key = String(value ?? '').trim().toLowerCase().replace(/_/g, '-');
  if (key === 'x-ai' || key === 'grok') return 'xai';
  if (key === 'google-vertex' || key === 'vertex-ai') return 'vertex';
  return key;
};

const getAuthFileProviderKey = (file: AuthFileItem): string =>
  normalizeProviderKey(file.type ?? file.provider);

const stringifySearchValue = (value: unknown): string[] => {
  if (value === undefined || value === null) return [];
  if (Array.isArray(value)) return value.flatMap(stringifySearchValue);
  if (typeof value === 'string') return value.trim() ? [value] : [];
  if (typeof value === 'number' || typeof value === 'boolean') return [String(value)];
  return [];
};

const matchesCredentialSearch = (file: AuthFileItem, normalizedSearchQuery: string): boolean => {
  if (!normalizedSearchQuery) return true;
  const values = [
    file.name,
    file.type,
    file.provider,
    file.authIndex,
    file['auth_index'],
    file.status,
    file.statusMessage,
    file['status_message'],
    file.account,
    file.email,
    file.projectId,
    file.project_id,
  ];
  return stringifySearchValue(values).some((value) =>
    value.toLowerCase().includes(normalizedSearchQuery)
  );
};

const getCredentialSubtitle = (file: AuthFileItem): string => {
  const candidates = [
    file.account,
    file.email,
    file.projectId,
    file.project_id,
    file.authIndex,
    file['auth_index'],
  ];
  for (const candidate of candidates) {
    const text = stringifySearchValue(candidate)[0];
    if (text) return text;
  }
  return '';
};

interface SupplementalProviderSectionProps {
  definition: SupplementalProviderDefinition;
  files: AuthFileItem[];
  disabled: boolean;
  searchQuery: string;
  onAction: (definition: SupplementalProviderDefinition) => void;
  getActionLabel: (definition: SupplementalProviderDefinition) => string;
}

function SupplementalProviderSection({
  definition,
  files,
  disabled,
  searchQuery,
  onAction,
  getActionLabel,
}: SupplementalProviderSectionProps) {
  const { t } = useTranslation();
  const normalizedSearchQuery = searchQuery.trim().toLowerCase();
  const providerFiles = useMemo(
    () =>
      files
        .filter((file) => getAuthFileProviderKey(file) === definition.id && file.disabled !== true)
        .sort((left, right) =>
          left.name.localeCompare(right.name, undefined, { numeric: true, sensitivity: 'base' })
        ),
    [definition.id, files]
  );
  const displayFiles = useMemo(
    () => providerFiles.filter((file) => matchesCredentialSearch(file, normalizedSearchQuery)),
    [normalizedSearchQuery, providerFiles]
  );
  const titleNode = (
    <div className={styles.titleWrapper}>
      <span>{t(definition.titleKey)}</span>
      {providerFiles.length > 0 && (
        <span className={styles.countBadge}>
          {normalizedSearchQuery ? displayFiles.length : providerFiles.length}
        </span>
      )}
    </div>
  );
  const isReserved = definition.action === 'reserved';

  return (
    <Card
      className={`${styles.providerSectionCard} ${styles.supplementalProviderCard} ${
        styles[`${definition.id}Card`]
      }`}
      id={`quota-provider-${definition.id}`}
      title={titleNode}
      extra={
        <div className={styles.headerActions}>
          <Button
            variant="secondary"
            size="sm"
            className={styles.loginCredentialButton}
            onClick={() => onAction(definition)}
            disabled={disabled || isReserved}
            title={isReserved ? t('kiro_quota.reserved_hint') : undefined}
          >
            {getActionLabel(definition)}
          </Button>
        </div>
      }
    >
      {providerFiles.length === 0 ? (
        <EmptyState title={t(definition.emptyTitleKey)} description={t(definition.emptyDescKey)} />
      ) : displayFiles.length === 0 ? (
        <EmptyState
          title={t('quota_management.search_empty_title')}
          description={t('quota_management.search_empty_desc')}
        />
      ) : (
        <div className={styles.supplementalCredentialSection}>
          <p className={styles.supplementalProviderHint}>{t(definition.hintKey)}</p>
          <div className={styles.supplementalCredentialList}>
            {displayFiles.map((file) => {
              const subtitle = getCredentialSubtitle(file);
              return (
                <div key={file.name} className={styles.supplementalCredentialItem}>
                  <div className={styles.supplementalCredentialMain}>
                    <span className={styles.supplementalCredentialName}>{file.name}</span>
                    {subtitle && (
                      <span className={styles.supplementalCredentialMeta}>{subtitle}</span>
                    )}
                  </div>
                  <span className={styles.supplementalCredentialType}>
                    {t(`auth_files.type_${definition.id}`, { defaultValue: definition.id })}
                  </span>
                </div>
              );
            })}
          </div>
        </div>
      )}
    </Card>
  );
}

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
      const titleKey = QUOTA_LOGIN_TITLE_KEY_MAP[type];
      return t('quota_management.login_provider', {
        provider: titleKey ? t(titleKey) : type,
      });
    },
    [t]
  );

  const openLoginModal = useCallback((type: QuotaType) => {
    const provider = QUOTA_OAUTH_PROVIDER_MAP[type];
    if (provider) {
      setLoginProvider(provider);
    }
  }, []);

  const getSupplementalActionLabel = useCallback(
    (definition: SupplementalProviderDefinition) => {
      if (definition.actionProviderTitleKey) {
        return t(definition.actionLabelKey, {
          provider: t(definition.actionProviderTitleKey),
        });
      }
      return t(definition.actionLabelKey);
    },
    [t]
  );

  const handleSupplementalAction = useCallback((definition: SupplementalProviderDefinition) => {
    if (definition.action === 'oauth') {
      setLoginProvider('xai');
      return;
    }
    if (definition.action === 'vertex-import') {
      setVertexImportOpen(true);
    }
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
      {SUPPLEMENTAL_PROVIDERS.map((definition) => (
        <SupplementalProviderSection
          key={definition.id}
          definition={definition}
          files={files}
          disabled={disableControls}
          searchQuery={searchQuery}
          onAction={handleSupplementalAction}
          getActionLabel={getSupplementalActionLabel}
        />
      ))}
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
