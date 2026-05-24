/**
 * 配额管理页面，负责统一加载认证文件、配置文件，并协调各供应商配额区块。
 */

import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useHeaderRefresh } from '@/hooks/useHeaderRefresh';
import { useAuthStore, useThemeStore } from '@/stores';
import { authFilesApi, configFileApi } from '@/services/api';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { EmptyState } from '@/components/ui/EmptyState';
import { Input } from '@/components/ui/Input';
import { Select } from '@/components/ui/Select';
import {
  IconRefreshCw,
  IconSearch,
} from '@/components/ui/icons';
import {
  QuotaCard,
  QuotaProviderNav,
  QuotaSection,
  ANTIGRAVITY_CONFIG,
  CLAUDE_CONFIG,
  CODEX_CONFIG,
  GEMINI_CLI_CONFIG,
  KIMI_CONFIG
} from '@/components/quota';
import { useGridColumns } from '@/components/quota/useGridColumns';
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

type SupplementalViewMode = 'paged' | 'all';

const SUPPLEMENTAL_GRID_MIN_WIDTH = 380;
const SUPPLEMENTAL_MAX_ITEMS_PER_PAGE = 25;
const SUPPLEMENTAL_MAX_SHOW_ALL_THRESHOLD = 30;

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

const SUPPLEMENTAL_CARD_CLASS_MAP: Record<SupplementalProviderId, string> = {
  xai: styles.xaiCard,
  vertex: styles.vertexCard,
  kiro: styles.kiroCard,
};

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

interface SupplementalProviderSectionProps {
  definition: SupplementalProviderDefinition;
  files: AuthFileItem[];
  loading: boolean;
  disabled: boolean;
  searchQuery: string;
  onRefresh: () => Promise<void>;
  onAction: (definition: SupplementalProviderDefinition) => void;
  getActionLabel: (definition: SupplementalProviderDefinition) => string;
}

function SupplementalProviderSection({
  definition,
  files,
  loading,
  disabled,
  searchQuery,
  onRefresh,
  onAction,
  getActionLabel,
}: SupplementalProviderSectionProps) {
  const { t } = useTranslation();
  const resolvedTheme = useThemeStore((state) => state.resolvedTheme);
  const [columns, gridRef] = useGridColumns(SUPPLEMENTAL_GRID_MIN_WIDTH);
  const [viewMode, setViewMode] = useState<SupplementalViewMode>('paged');
  const [page, setPage] = useState(1);
  const [showTooManyWarning, setShowTooManyWarning] = useState(false);
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
  const showAllAllowed = displayFiles.length <= SUPPLEMENTAL_MAX_SHOW_ALL_THRESHOLD;
  const effectiveViewMode: SupplementalViewMode =
    viewMode === 'all' && showAllAllowed ? 'all' : 'paged';
  const pageSize =
    effectiveViewMode === 'all'
      ? Math.max(1, displayFiles.length)
      : Math.min(columns * 3, SUPPLEMENTAL_MAX_ITEMS_PER_PAGE);
  const totalPages = Math.max(1, Math.ceil(displayFiles.length / pageSize));
  const currentPage = Math.min(page, totalPages);
  const pageItems = useMemo(() => {
    if (effectiveViewMode === 'all') return displayFiles;
    const start = (currentPage - 1) * pageSize;
    return displayFiles.slice(start, start + pageSize);
  }, [currentPage, displayFiles, effectiveViewMode, pageSize]);

  useEffect(() => {
    setPage(1);
  }, [definition.id, displayFiles.length, effectiveViewMode, normalizedSearchQuery, pageSize]);

  useEffect(() => {
    if (showAllAllowed) return;
    if (viewMode !== 'all') return;

    let cancelled = false;
    queueMicrotask(() => {
      if (cancelled) return;
      setViewMode('paged');
      setShowTooManyWarning(true);
    });

    return () => {
      cancelled = true;
    };
  }, [showAllAllowed, viewMode]);

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
  const isRefreshing = loading;

  return (
    <Card
      className={styles.providerSectionCard}
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
          <div className={styles.viewModeToggle}>
            <Button
              variant="secondary"
              size="sm"
              className={`${styles.viewModeButton} ${
                effectiveViewMode === 'paged' ? styles.viewModeButtonActive : ''
              }`}
              onClick={() => setViewMode('paged')}
            >
              {t('auth_files.view_mode_paged')}
            </Button>
            <Button
              variant="secondary"
              size="sm"
              className={`${styles.viewModeButton} ${
                effectiveViewMode === 'all' ? styles.viewModeButtonActive : ''
              }`}
              onClick={() => {
                if (showAllAllowed) {
                  setViewMode('all');
                } else {
                  setShowTooManyWarning(true);
                }
              }}
            >
              {t('auth_files.view_mode_all')}
            </Button>
          </div>
          <Button
            variant="secondary"
            size="sm"
            className={styles.refreshAllButton}
            onClick={() => void onRefresh()}
            disabled={disabled || isRefreshing}
            loading={isRefreshing}
            title={t('quota_management.refresh_all_credentials')}
            aria-label={t('quota_management.refresh_all_credentials')}
          >
            {!isRefreshing && <IconRefreshCw size={16} />}
            {t('quota_management.refresh_all_credentials')}
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
        <>
          <div ref={gridRef} className={styles.supplementalCredentialGrid}>
            {pageItems.map((item) => (
              <QuotaCard
                key={item.name}
                item={item}
                resolvedTheme={resolvedTheme}
                i18nPrefix={`${definition.id}_quota`}
                cardIdleMessageKey={definition.hintKey}
                cardClassName={SUPPLEMENTAL_CARD_CLASS_MAP[definition.id]}
                defaultType={definition.id}
                renderQuotaItems={() => null}
              />
            ))}
          </div>
          {displayFiles.length > pageSize && effectiveViewMode === 'paged' && (
            <div className={styles.pagination}>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => setPage((prev) => Math.max(1, prev - 1))}
                disabled={currentPage <= 1}
              >
                {t('auth_files.pagination_prev')}
              </Button>
              <div className={styles.pageInfo}>
                {t('auth_files.pagination_info', {
                  current: currentPage,
                  total: totalPages,
                  count: displayFiles.length
                })}
              </div>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => setPage((prev) => Math.min(totalPages, prev + 1))}
                disabled={currentPage >= totalPages}
              >
                {t('auth_files.pagination_next')}
              </Button>
            </div>
          )}
        </>
      )}
      {showTooManyWarning && (
        <div className={styles.warningOverlay} onClick={() => setShowTooManyWarning(false)}>
          <div className={styles.warningModal} onClick={(e) => e.stopPropagation()}>
            <p>{t('auth_files.too_many_files_warning')}</p>
            <Button variant="primary" size="sm" onClick={() => setShowTooManyWarning(false)}>
              {t('common.confirm')}
            </Button>
          </div>
        </div>
      )}
    </Card>
  );
}

export function QuotaPage() {
  const { t } = useTranslation();
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
          loading={loading}
          disabled={disableControls}
          searchQuery={searchQuery}
          onRefresh={handleHeaderRefresh}
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
