import { useCallback, useEffect, useMemo, useState, type CSSProperties, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import { EmptyState } from '@/components/ui/EmptyState';
import { Input } from '@/components/ui/Input';
import { Modal } from '@/components/ui/Modal';
import { Select, type SelectOption } from '@/components/ui/Select';
import { IconRefreshCw, IconSearch, IconTrash2 } from '@/components/ui/icons';
import { authFilesApi } from '@/services/api';
import { useNotificationStore, useThemeStore } from '@/stores';
import type { AuthFileItem } from '@/types';
import {
  getTypeColor,
  getTypeLabel,
  hasAuthFileStatusMessage,
  normalizeProviderKey,
  type ResolvedTheme,
} from '@/features/authFiles/constants';
import { useHeaderRefresh } from '@/hooks/useHeaderRefresh';
import styles from './AccountGroupsPage.module.scss';

const UNGROUPED_VALUE = '__ungrouped__';

type AccountStatusFilter = 'all' | 'active' | 'disabled' | 'unavailable';

type AccountGroupView = {
  key: string;
  name: string;
  isUngrouped: boolean;
  files: AuthFileItem[];
  active: number;
  disabled: number;
  unavailable: number;
  providers: Array<{ provider: string; count: number }>;
};

type AccountGroupUpdate = {
  file: AuthFileItem;
  groups: string[];
};

const readText = (value: unknown): string => {
  if (typeof value === 'string') return value.trim();
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  return '';
};

const normalizeGroupInput = (value: string): string => value.trim().replace(/\s+/g, ' ');

const normalizeGroupList = (groups: string[]): string[] => {
  const seen = new Set<string>();
  return groups.reduce<string[]>((result, group) => {
    const normalized = normalizeGroupInput(group);
    if (!normalized || seen.has(normalized)) return result;
    seen.add(normalized);
    result.push(normalized);
    return result;
  }, []);
};

const readStringArray = (value: unknown): string[] => {
  if (Array.isArray(value)) {
    return value
      .map((item) => readText(item))
      .filter(Boolean);
  }
  return [];
};

const resolveAccountGroups = (file: AuthFileItem): string[] =>
  normalizeGroupList([
    ...readStringArray(file.account_groups),
    ...readStringArray(file.accountGroups),
    readText(file.account_group),
    readText(file.accountGroup),
    readText(file.group),
  ]);

const resolveProvider = (file: AuthFileItem): string => {
  const provider = readText(file.provider) || readText(file.type);
  return normalizeProviderKey(provider || 'unknown');
};

const resolveAccountLabel = (file: AuthFileItem): string =>
  readText(file.email) ||
  readText(file.account) ||
  readText(file.project_id) ||
  readText(file.projectId) ||
  file.name;

const resolveProjectLabel = (file: AuthFileItem): string =>
  readText(file.project_id) || readText(file.projectId) || readText(file.gemini_virtual_project);

const isUnavailableAccount = (file: AuthFileItem): boolean =>
  file.unavailable === true || hasAuthFileStatusMessage(file);

const matchesStatusFilter = (file: AuthFileItem, filter: AccountStatusFilter): boolean => {
  if (filter === 'all') return true;
  if (filter === 'disabled') return file.disabled === true;
  if (filter === 'unavailable') return file.disabled !== true && isUnavailableAccount(file);
  return file.disabled !== true && !isUnavailableAccount(file);
};

const sortByName = (left: AuthFileItem, right: AuthFileItem) =>
  left.name.localeCompare(right.name, undefined, { numeric: true, sensitivity: 'base' });

export function AccountGroupsPage() {
  const { t } = useTranslation();
  const showNotification = useNotificationStore((state) => state.showNotification);
  const showConfirmation = useNotificationStore((state) => state.showConfirmation);
  const resolvedTheme: ResolvedTheme = useThemeStore((state) => state.resolvedTheme);

  const [files, setFiles] = useState<AuthFileItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [search, setSearch] = useState('');
  const [providerFilter, setProviderFilter] = useState('all');
  const [statusFilter, setStatusFilter] = useState<AccountStatusFilter>('all');
  const [updatingNames, setUpdatingNames] = useState<Set<string>>(new Set());
  const [actionBusy, setActionBusy] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [createGroupName, setCreateGroupName] = useState('');
  const [createSelected, setCreateSelected] = useState<Set<string>>(new Set());
  const [renameTarget, setRenameTarget] = useState<AccountGroupView | null>(null);
  const [renameGroupName, setRenameGroupName] = useState('');
  const [editTarget, setEditTarget] = useState<AuthFileItem | null>(null);
  const [editSelected, setEditSelected] = useState<Set<string>>(new Set());

  const loadFiles = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const payload = await authFilesApi.list();
      setFiles([...(payload.files ?? [])].sort(sortByName));
    } catch (err) {
      const message = err instanceof Error ? err.message : t('account_groups.load_failed');
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    void loadFiles();
  }, [loadFiles]);

  useHeaderRefresh(loadFiles);

  const providerOptions = useMemo<SelectOption[]>(() => {
    const providers = Array.from(new Set(files.map(resolveProvider))).sort((left, right) =>
      getTypeLabel(t, left).localeCompare(getTypeLabel(t, right), undefined, {
        sensitivity: 'base',
      })
    );

    return [
      { value: 'all', label: t('account_groups.all_providers') },
      ...providers.map((provider) => ({ value: provider, label: getTypeLabel(t, provider) })),
    ];
  }, [files, t]);

  const statusOptions = useMemo<SelectOption[]>(
    () => [
      { value: 'all', label: t('account_groups.status_all') },
      { value: 'active', label: t('account_groups.status_active') },
      { value: 'disabled', label: t('account_groups.status_disabled') },
      { value: 'unavailable', label: t('account_groups.status_unavailable') },
    ],
    [t]
  );

  const filteredFiles = useMemo(() => {
    const query = search.trim().toLowerCase();
    return files.filter((file) => {
      const provider = resolveProvider(file);
      if (providerFilter !== 'all' && provider !== providerFilter) return false;
      if (!matchesStatusFilter(file, statusFilter)) return false;
      if (!query) return true;

      const values = [
        file.name,
        resolveAccountLabel(file),
        resolveProjectLabel(file),
        resolveAccountGroups(file).join(' '),
        readText(file.note),
        readText(file.status),
        readText(file.statusMessage),
        readText(file['status_message']),
        getTypeLabel(t, provider),
      ];
      return values.some((value) => value.toLowerCase().includes(query));
    });
  }, [files, providerFilter, search, statusFilter, t]);

  const groups = useMemo<AccountGroupView[]>(() => {
    const grouped = new Map<string, AuthFileItem[]>();
    filteredFiles.forEach((file) => {
      const accountGroups = resolveAccountGroups(file);
      const keys = accountGroups.length > 0 ? accountGroups : [UNGROUPED_VALUE];
      keys.forEach((key) => {
        const bucket = grouped.get(key);
        if (bucket) {
          bucket.push(file);
          return;
        }
        grouped.set(key, [file]);
      });
    });

    return Array.from(grouped.entries())
      .map(([key, groupFiles]) => {
        const isUngrouped = key === UNGROUPED_VALUE;
        const providerCounts = new Map<string, number>();
        let active = 0;
        let disabled = 0;
        let unavailable = 0;

        groupFiles.forEach((file) => {
          const provider = resolveProvider(file);
          providerCounts.set(provider, (providerCounts.get(provider) ?? 0) + 1);
          if (file.disabled === true) {
            disabled += 1;
          } else if (isUnavailableAccount(file)) {
            unavailable += 1;
          } else {
            active += 1;
          }
        });

        const providers = Array.from(providerCounts.entries())
          .map(([provider, count]) => ({ provider, count }))
          .sort((left, right) => right.count - left.count || left.provider.localeCompare(right.provider));

        return {
          key,
          name: isUngrouped ? t('account_groups.ungrouped') : key,
          isUngrouped,
          files: [...groupFiles].sort(sortByName),
          active,
          disabled,
          unavailable,
          providers,
        };
      })
      .sort((left, right) => {
        if (left.isUngrouped !== right.isUngrouped) return left.isUngrouped ? -1 : 1;
        return left.name.localeCompare(right.name, undefined, { numeric: true, sensitivity: 'base' });
      });
  }, [filteredFiles, t]);

  const groupNames = useMemo(
    () =>
      Array.from(new Set(files.flatMap(resolveAccountGroups).filter(Boolean))).sort((left, right) =>
        left.localeCompare(right, undefined, { numeric: true, sensitivity: 'base' })
      ),
    [files]
  );

  const totalActive = files.filter((file) => file.disabled !== true && !isUnavailableAccount(file)).length;
  const totalDisabled = files.filter((file) => file.disabled === true).length;
  const totalUnavailable = files.filter(
    (file) => file.disabled !== true && isUnavailableAccount(file)
  ).length;

  const markUpdating = useCallback((names: string[], updating: boolean) => {
    setUpdatingNames((prev) => {
      const next = new Set(prev);
      names.forEach((name) => {
        if (updating) {
          next.add(name);
        } else {
          next.delete(name);
        }
      });
      return next;
    });
  }, []);

  const saveAccountGroupUpdates = useCallback(
    async (updates: AccountGroupUpdate[]) => {
      const normalizedUpdates = updates
        .map((update) => ({
          file: update.file,
          groups: normalizeGroupList(update.groups),
        }))
        .filter((update) => update.file.name);
      const names = Array.from(new Set(normalizedUpdates.map((update) => update.file.name)));
      if (names.length === 0) return;

      setActionBusy(true);
      markUpdating(names, true);
      try {
        await Promise.all(
          normalizedUpdates.map((update) =>
            authFilesApi.patchFields(update.file.name, { account_groups: update.groups })
          )
        );
        await loadFiles();
        showNotification(t('account_groups.save_success'), 'success');
      } catch (err) {
        const message = err instanceof Error ? err.message : t('account_groups.save_failed');
        showNotification(`${t('account_groups.save_failed')}: ${message}`, 'error');
      } finally {
        markUpdating(names, false);
        setActionBusy(false);
      }
    },
    [loadFiles, markUpdating, showNotification, t]
  );

  const getFilesInGroup = useCallback(
    (groupName: string): AuthFileItem[] =>
      files.filter((file) => resolveAccountGroups(file).includes(groupName)),
    [files]
  );

  const toggleCreateSelection = useCallback((name: string) => {
    setCreateSelected((prev) => {
      const next = new Set(prev);
      if (next.has(name)) {
        next.delete(name);
      } else {
        next.add(name);
      }
      return next;
    });
  }, []);

  const openCreateModal = useCallback(() => {
    setCreateGroupName('');
    setCreateSelected(new Set());
    setCreateOpen(true);
  }, []);

  const submitCreateGroup = useCallback(
    async (event: FormEvent<HTMLFormElement>) => {
      event.preventDefault();
      const groupName = normalizeGroupInput(createGroupName);
      if (!groupName) {
        showNotification(t('account_groups.group_name_required'), 'warning');
        return;
      }
      if (createSelected.size === 0) {
        showNotification(t('account_groups.create_select_required'), 'warning');
        return;
      }

      const selectedFiles = files.filter((file) => createSelected.has(file.name));
      await saveAccountGroupUpdates(
        selectedFiles.map((file) => ({
          file,
          groups: [...resolveAccountGroups(file), groupName],
        }))
      );
      setCreateOpen(false);
    },
    [createGroupName, createSelected, files, saveAccountGroupUpdates, showNotification, t]
  );

  const openRenameModal = useCallback((group: AccountGroupView) => {
    setRenameTarget(group);
    setRenameGroupName(group.isUngrouped ? '' : group.name);
  }, []);

  const submitRenameGroup = useCallback(
    async (event: FormEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (!renameTarget) return;
      const nextGroup = normalizeGroupInput(renameGroupName);
      if (!nextGroup) {
        showNotification(t('account_groups.group_name_required'), 'warning');
        return;
      }
      if (!renameTarget.isUngrouped && nextGroup === renameTarget.name) {
        setRenameTarget(null);
        return;
      }

      const targetFiles = getFilesInGroup(renameTarget.key);
      await saveAccountGroupUpdates(
        targetFiles.map((file) => ({
          file,
          groups: resolveAccountGroups(file).map((groupName) =>
            groupName === renameTarget.key ? nextGroup : groupName
          ),
        }))
      );
      setRenameTarget(null);
    },
    [getFilesInGroup, renameGroupName, renameTarget, saveAccountGroupUpdates, showNotification, t]
  );

  const clearGroup = useCallback(
    (group: AccountGroupView) => {
      showConfirmation({
        title: t('account_groups.clear_group'),
        message: t('account_groups.clear_group_confirm', { group: group.name }),
        confirmText: t('account_groups.clear_group'),
        cancelText: t('common.cancel'),
        variant: 'danger',
        onConfirm: async () => {
          const targetFiles = getFilesInGroup(group.key);
          await saveAccountGroupUpdates(
            targetFiles.map((file) => ({
              file,
              groups: resolveAccountGroups(file).filter((groupName) => groupName !== group.key),
            }))
          );
        },
      });
    },
    [getFilesInGroup, saveAccountGroupUpdates, showConfirmation, t]
  );

  const openEditGroupsModal = useCallback((file: AuthFileItem) => {
    setEditTarget(file);
    setEditSelected(new Set(resolveAccountGroups(file)));
  }, []);

  const toggleEditSelection = useCallback((groupName: string) => {
    setEditSelected((prev) => {
      const next = new Set(prev);
      if (next.has(groupName)) {
        next.delete(groupName);
      } else {
        next.add(groupName);
      }
      return next;
    });
  }, []);

  const submitEditGroups = useCallback(
    async (event: FormEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (!editTarget) return;
      await saveAccountGroupUpdates([{ file: editTarget, groups: Array.from(editSelected) }]);
      setEditTarget(null);
    },
    [editSelected, editTarget, saveAccountGroupUpdates]
  );

  const renderProviderBadge = (provider: string, count?: number) => {
    const color = getTypeColor(provider, resolvedTheme);
    const style: CSSProperties = {
      backgroundColor: color.bg,
      color: color.text,
      border: color.border,
    };

    return (
      <span className={styles.providerBadge} style={style}>
        {getTypeLabel(t, provider)}
        {typeof count === 'number' && <span className={styles.providerCount}>{count}</span>}
      </span>
    );
  };

  return (
    <div className={styles.container}>
      <header className={styles.pageHeader}>
        <div>
          <h1 className={styles.pageTitle}>{t('account_groups.title')}</h1>
          <p className={styles.description}>{t('account_groups.description')}</p>
        </div>
        <div className={styles.headerActions}>
          <Button variant="secondary" onClick={() => void loadFiles()} loading={loading}>
            <IconRefreshCw size={16} />
            {t('common.refresh')}
          </Button>
          <Button onClick={openCreateModal}>{t('account_groups.create_group')}</Button>
        </div>
      </header>

      <section className={styles.summaryGrid} aria-label={t('account_groups.summary')}>
        <div className={styles.summaryItem}>
          <span>{t('account_groups.total_accounts')}</span>
          <strong>{files.length}</strong>
        </div>
        <div className={styles.summaryItem}>
          <span>{t('account_groups.total_groups')}</span>
          <strong>{groupNames.length}</strong>
        </div>
        <div className={styles.summaryItem}>
          <span>{t('account_groups.status_active')}</span>
          <strong>{totalActive}</strong>
        </div>
        <div className={styles.summaryItem}>
          <span>{t('account_groups.status_unavailable')}</span>
          <strong>{totalUnavailable}</strong>
        </div>
        <div className={styles.summaryItem}>
          <span>{t('account_groups.status_disabled')}</span>
          <strong>{totalDisabled}</strong>
        </div>
      </section>

      <section className={styles.toolbar}>
        <Input
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          placeholder={t('account_groups.search_placeholder')}
          aria-label={t('account_groups.search_placeholder')}
          rightElement={<IconSearch size={16} />}
        />
        <Select
          value={providerFilter}
          options={providerOptions}
          onChange={setProviderFilter}
          ariaLabel={t('account_groups.provider_filter')}
        />
        <Select
          value={statusFilter}
          options={statusOptions}
          onChange={(value) => setStatusFilter(value as AccountStatusFilter)}
          ariaLabel={t('account_groups.status_filter')}
        />
      </section>

      {error && <div className={styles.errorBox}>{error}</div>}

      {loading ? (
        <div className={styles.loadingBox}>{t('account_groups.loading')}</div>
      ) : groups.length === 0 ? (
        <EmptyState
          title={t('account_groups.empty_title')}
          description={t('account_groups.empty_description')}
          action={<Button onClick={openCreateModal}>{t('account_groups.create_group')}</Button>}
        />
      ) : (
        <section className={styles.groupList}>
          {groups.map((group) => (
            <article className={styles.groupCard} key={group.key}>
              <div className={styles.groupHeader}>
                <div className={styles.groupTitleWrap}>
                  <h2 className={styles.groupTitle}>{group.name}</h2>
                  <span className={styles.countBadge}>
                    {t('account_groups.accounts_count', { count: group.files.length })}
                  </span>
                </div>
                <div className={styles.groupActions}>
                  {!group.isUngrouped && (
                    <Button
                      variant="secondary"
                      size="sm"
                      onClick={() => openRenameModal(group)}
                      disabled={actionBusy}
                    >
                      {t('account_groups.rename_group')}
                    </Button>
                  )}
                  {!group.isUngrouped && (
                    <Button
                      variant="danger"
                      size="sm"
                      onClick={() => clearGroup(group)}
                      disabled={actionBusy}
                    >
                      <IconTrash2 size={14} />
                      {t('account_groups.clear_group')}
                    </Button>
                  )}
                </div>
              </div>

              <div className={styles.groupMeta}>
                <span>{t('account_groups.status_active')}: {group.active}</span>
                <span>{t('account_groups.status_unavailable')}: {group.unavailable}</span>
                <span>{t('account_groups.status_disabled')}: {group.disabled}</span>
              </div>

              <div className={styles.providerRow}>
                {group.providers.slice(0, 6).map((entry) => (
                  <span key={entry.provider}>{renderProviderBadge(entry.provider, entry.count)}</span>
                ))}
              </div>

              <div className={styles.accountTable}>
                {group.files.map((file) => {
                  const provider = resolveProvider(file);
                  const statusLabel =
                    file.disabled === true
                      ? t('account_groups.status_disabled')
                      : isUnavailableAccount(file)
                        ? t('account_groups.status_unavailable')
                        : t('account_groups.status_active');
                  const statusClass =
                    file.disabled === true
                      ? styles.statusDisabled
                      : isUnavailableAccount(file)
                        ? styles.statusUnavailable
                        : styles.statusActive;
                  const accountGroups = resolveAccountGroups(file);

                  return (
                    <div className={styles.accountRow} key={file.name}>
                      <div className={styles.accountMain}>
                        <div className={styles.accountName}>{file.name}</div>
                        <div className={styles.accountSub}>
                          <span>{resolveAccountLabel(file)}</span>
                          {resolveProjectLabel(file) && <span>{resolveProjectLabel(file)}</span>}
                        </div>
                      </div>
                      <div className={styles.accountProvider}>{renderProviderBadge(provider)}</div>
                      <span className={`${styles.statusBadge} ${statusClass}`}>{statusLabel}</span>
                      <div className={styles.accountGroupsCell}>
                        <div className={styles.accountGroupChips}>
                          {accountGroups.length > 0 ? (
                            accountGroups.map((groupName) => (
                              <span className={styles.groupChip} key={groupName}>
                                {groupName}
                              </span>
                            ))
                          ) : (
                            <span className={styles.ungroupedChip}>{t('account_groups.ungrouped')}</span>
                          )}
                        </div>
                        <Button
                          variant="secondary"
                          size="sm"
                          onClick={() => openEditGroupsModal(file)}
                          disabled={updatingNames.has(file.name)}
                        >
                          {t('account_groups.edit_membership')}
                        </Button>
                      </div>
                    </div>
                  );
                })}
              </div>
            </article>
          ))}
        </section>
      )}

      <Modal
        open={createOpen}
        title={t('account_groups.create_group')}
        onClose={() => setCreateOpen(false)}
        width={720}
        footer={
          <>
            <Button variant="secondary" onClick={() => setCreateOpen(false)} disabled={actionBusy}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" form="account-group-create-form" loading={actionBusy}>
              {t('common.save')}
            </Button>
          </>
        }
      >
        <form id="account-group-create-form" className={styles.modalForm} onSubmit={submitCreateGroup}>
          <Input
            value={createGroupName}
            onChange={(event) => setCreateGroupName(event.target.value)}
            label={t('account_groups.group_name')}
            placeholder={t('account_groups.group_name_placeholder')}
            autoFocus
          />
          <div className={styles.modalAccountsHeader}>
            <span>{t('account_groups.select_accounts')}</span>
            <span>{t('account_groups.selected_count', { count: createSelected.size })}</span>
          </div>
          <div className={styles.modalAccountList}>
            {files.map((file) => {
              const selected = createSelected.has(file.name);
              return (
                <label className={styles.modalAccountItem} key={file.name}>
                  <input
                    type="checkbox"
                    checked={selected}
                    onChange={() => toggleCreateSelection(file.name)}
                  />
                  <span>
                    <strong>{file.name}</strong>
                    <small>{resolveAccountLabel(file)}</small>
                  </span>
                </label>
              );
            })}
          </div>
        </form>
      </Modal>

      <Modal
        open={Boolean(editTarget)}
        title={t('account_groups.edit_membership')}
        onClose={() => setEditTarget(null)}
        width={560}
        footer={
          <>
            <Button variant="secondary" onClick={() => setEditTarget(null)} disabled={actionBusy}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" form="account-group-edit-form" loading={actionBusy}>
              {t('common.save')}
            </Button>
          </>
        }
      >
        <form id="account-group-edit-form" className={styles.modalForm} onSubmit={submitEditGroups}>
          {editTarget && (
            <div className={styles.editAccountSummary}>
              <strong>{editTarget.name}</strong>
              <span>{resolveAccountLabel(editTarget)}</span>
            </div>
          )}
          {groupNames.length === 0 ? (
            <p className={styles.renameHint}>{t('account_groups.no_groups_hint')}</p>
          ) : (
            <>
              <div className={styles.modalAccountsHeader}>
                <span>{t('account_groups.group_memberships')}</span>
                <span>{t('account_groups.selected_count', { count: editSelected.size })}</span>
              </div>
              <div className={styles.modalAccountList}>
                {groupNames.map((groupName) => (
                  <label className={styles.modalAccountItem} key={groupName}>
                    <input
                      type="checkbox"
                      checked={editSelected.has(groupName)}
                      onChange={() => toggleEditSelection(groupName)}
                    />
                    <span>
                      <strong>{groupName}</strong>
                      <small>{t('account_groups.group_membership_hint')}</small>
                    </span>
                  </label>
                ))}
              </div>
            </>
          )}
        </form>
      </Modal>

      <Modal
        open={Boolean(renameTarget)}
        title={t('account_groups.rename_group')}
        onClose={() => setRenameTarget(null)}
        width={520}
        footer={
          <>
            <Button variant="secondary" onClick={() => setRenameTarget(null)} disabled={actionBusy}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" form="account-group-rename-form" loading={actionBusy}>
              {t('common.save')}
            </Button>
          </>
        }
      >
        <form id="account-group-rename-form" className={styles.modalForm} onSubmit={submitRenameGroup}>
          <Input
            value={renameGroupName}
            onChange={(event) => setRenameGroupName(event.target.value)}
            label={t('account_groups.group_name')}
            placeholder={t('account_groups.group_name_placeholder')}
            autoFocus
          />
          {renameTarget && (
            <p className={styles.renameHint}>
              {t('account_groups.rename_hint', { count: getFilesInGroup(renameTarget.key).length })}
            </p>
          )}
        </form>
      </Modal>
    </div>
  );
}
