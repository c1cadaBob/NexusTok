/*
Copyright (C) 2025 c1cada

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@c1cada.dev
*/

import React, { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Card,
  Form,
  Input,
  Modal,
  Select,
  Space,
  Table,
  Tag,
  TextArea,
  Typography,
} from '@douyinfe/semi-ui';
import { API, showError, showSuccess } from '../../helpers';

const { Text, Title } = Typography;

const ACCOUNT_STATUS = {
  ENABLED: 1,
  MANUAL_DISABLED: 2,
  AUTO_DISABLED: 3,
};

const defaultStats = {
  total: 0,
  enabled: 0,
  disabled: 0,
  cooldown: 0,
};

const emptyGroupForm = {
  id: undefined,
  name: '',
  platform: 'codex',
  auth_type: 'official_oauth',
  strategy: 'round_robin',
  models: '',
  group: '',
  model_mapping: '',
  settings: '',
  status: ACCOUNT_STATUS.ENABLED,
};

const emptyAccountForm = {
  id: undefined,
  name: '',
  credentials: '',
  platform: '',
  auth_type: '',
  models: '',
  group: '',
  priority: 0,
  weight: 1,
  max_concurrency: 0,
  proxy: '',
  status: ACCOUNT_STATUS.ENABLED,
};

function getCoolingUntil(account) {
  return Math.max(
    Number(account?.rate_limited_until || 0),
    Number(account?.overload_until || 0),
    Number(account?.temp_disabled_until || 0),
  );
}

function formatUnixTime(value) {
  const timestamp = Number(value || 0);
  if (!timestamp) return '-';
  return new Date(timestamp * 1000).toLocaleString();
}

function getAccountStatus(account, t) {
  const now = Math.floor(Date.now() / 1000);
  if (
    account.status !== ACCOUNT_STATUS.ENABLED ||
    account.schedulable === false
  ) {
    return {
      color: account.status === ACCOUNT_STATUS.AUTO_DISABLED ? 'yellow' : 'red',
      text:
        account.status === ACCOUNT_STATUS.AUTO_DISABLED
          ? t('自动禁用')
          : t('已禁用'),
    };
  }
  if (getCoolingUntil(account) > now) {
    return { color: 'orange', text: t('冷却中') };
  }
  return { color: 'green', text: t('已启用') };
}

function formatCredentialSummary(summary) {
  if (!summary) return '-';
  try {
    const parsed = JSON.parse(summary);
    return Object.entries(parsed)
      .map(([key, value]) => `${key}: ${value}`)
      .join(' | ');
  } catch {
    return summary;
  }
}

const AccountPool = () => {
  const { t } = useTranslation();
  const [groups, setGroups] = useState([]);
  const [selectedGroupId, setSelectedGroupId] = useState();
  const [accounts, setAccounts] = useState([]);
  const [stats, setStats] = useState(defaultStats);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [statusFilter, setStatusFilter] = useState('all');
  const [search, setSearch] = useState('');
  const [loading, setLoading] = useState(false);
  const [actionLoading, setActionLoading] = useState(false);
  const [groupFormVisible, setGroupFormVisible] = useState(false);
  const [groupForm, setGroupForm] = useState(emptyGroupForm);
  const [accountFormVisible, setAccountFormVisible] = useState(false);
  const [accountForm, setAccountForm] = useState(emptyAccountForm);
  const [batchVisible, setBatchVisible] = useState(false);
  const [batchCredentials, setBatchCredentials] = useState('');
  const [oauthVisible, setOauthVisible] = useState(false);
  const [oauthInput, setOauthInput] = useState('');
  const [oauthName, setOauthName] = useState('');

  const selectedGroup = useMemo(
    () => groups.find((group) => group.id === selectedGroupId),
    [groups, selectedGroupId],
  );

  const queryParams = useMemo(() => {
    const params = {
      p: page,
      page_size: pageSize,
    };
    if (search.trim()) {
      params.search = search.trim();
    }
    if (statusFilter !== 'all') {
      params.status = Number(statusFilter);
    }
    return params;
  }, [page, pageSize, search, statusFilter]);

  const loadGroups = async () => {
    try {
      const res = await API.get('/api/account-pool/groups', {
        params: { p: 1, page_size: 100 },
      });
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('获取账号池失败'));
        return;
      }
      const items = data?.items || [];
      setGroups(items);
      if (!selectedGroupId && items.length > 0) {
        setSelectedGroupId(items[0].id);
      } else if (
        selectedGroupId &&
        items.length > 0 &&
        !items.some((group) => group.id === selectedGroupId)
      ) {
        setSelectedGroupId(items[0].id);
      }
    } catch (error) {
      showError(error?.message || t('获取账号池失败'));
    }
  };

  const loadAccounts = async () => {
    if (!selectedGroupId) {
      setAccounts([]);
      setStats(defaultStats);
      setTotal(0);
      return;
    }
    setLoading(true);
    try {
      const res = await API.get(
        `/api/account-pool/groups/${selectedGroupId}/accounts`,
        { params: queryParams },
      );
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('获取账号池失败'));
        return;
      }
      setAccounts(data?.accounts?.items || []);
      setTotal(data?.accounts?.total || 0);
      setStats(data?.stats || defaultStats);
    } catch (error) {
      showError(error?.message || t('获取账号池失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadGroups();
  }, []);

  useEffect(() => {
    loadAccounts();
  }, [selectedGroupId, queryParams]);

  const refreshAll = async () => {
    await loadGroups();
    await loadAccounts();
  };

  const openCreateGroup = () => {
    setGroupForm(emptyGroupForm);
    setGroupFormVisible(true);
  };

  const openEditGroup = (group) => {
    setGroupForm({
      id: group.id,
      name: group.name || '',
      platform: group.platform || 'codex',
      auth_type: group.auth_type || 'official_oauth',
      strategy: group.strategy || 'round_robin',
      models: group.models || '',
      group: group.group || '',
      model_mapping: group.model_mapping || '',
      settings: group.settings || '',
      status: group.status || ACCOUNT_STATUS.ENABLED,
    });
    setGroupFormVisible(true);
  };

  const submitGroup = async () => {
    if (!groupForm.name.trim()) {
      showError(t('请输入名称'));
      return;
    }
    setActionLoading(true);
    try {
      const payload = {
        name: groupForm.name.trim(),
        platform: groupForm.platform.trim(),
        auth_type: groupForm.auth_type,
        strategy: groupForm.strategy,
        models: groupForm.models.trim(),
        group: groupForm.group.trim(),
        model_mapping: groupForm.model_mapping.trim(),
        settings: groupForm.settings.trim(),
        status: Number(groupForm.status || ACCOUNT_STATUS.ENABLED),
      };
      const res = groupForm.id
        ? await API.put(`/api/account-pool/groups/${groupForm.id}`, payload)
        : await API.post('/api/account-pool/groups', payload);
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('操作失败'));
        return;
      }
      showSuccess(t('操作成功完成！'));
      setGroupFormVisible(false);
      await loadGroups();
      if (!groupForm.id && data?.id) {
        setSelectedGroupId(data.id);
      }
    } catch (error) {
      showError(error?.message || t('操作失败'));
    } finally {
      setActionLoading(false);
    }
  };

  const deleteGroup = (group) => {
    Modal.confirm({
      title: t('删除账号组'),
      content: t('确定要删除该账号组和组内账号吗？'),
      onOk: async () => {
        setActionLoading(true);
        try {
          const res = await API.delete(`/api/account-pool/groups/${group.id}`);
          const { success, message } = res.data || {};
          if (!success) {
            showError(message || t('操作失败'));
            return;
          }
          showSuccess(t('操作成功完成！'));
          if (selectedGroupId === group.id) {
            setSelectedGroupId(undefined);
          }
          await loadGroups();
        } catch (error) {
          showError(error?.message || t('操作失败'));
        } finally {
          setActionLoading(false);
        }
      },
    });
  };

  const openCreateAccount = () => {
    setAccountForm({
      ...emptyAccountForm,
      platform: selectedGroup?.platform || '',
      auth_type: selectedGroup?.auth_type || '',
    });
    setAccountFormVisible(true);
  };

  const openEditAccount = (account) => {
    setAccountForm({
      id: account.id,
      name: account.name || '',
      credentials: '',
      platform: account.platform || selectedGroup?.platform || '',
      auth_type: account.auth_type || selectedGroup?.auth_type || '',
      models: account.models || '',
      group: account.group || '',
      priority: account.priority || 0,
      weight: account.weight || 1,
      max_concurrency: account.max_concurrency || 0,
      proxy: account.proxy || '',
      status: account.status || ACCOUNT_STATUS.ENABLED,
    });
    setAccountFormVisible(true);
  };

  const submitAccount = async () => {
    if (!selectedGroupId) return;
    if (!accountForm.name.trim()) {
      showError(t('请输入名称'));
      return;
    }
    if (!accountForm.id && !accountForm.credentials.trim()) {
      showError(t('请填写账号凭证'));
      return;
    }
    setActionLoading(true);
    try {
      const payload = {
        name: accountForm.name.trim(),
        credentials: accountForm.credentials.trim(),
        platform: accountForm.platform.trim(),
        auth_type: accountForm.auth_type,
        models: accountForm.models.trim(),
        group: accountForm.group.trim(),
        priority: Number(accountForm.priority || 0),
        weight: Number(accountForm.weight || 1),
        max_concurrency: Number(accountForm.max_concurrency || 0),
        proxy: accountForm.proxy.trim(),
        status: Number(accountForm.status || ACCOUNT_STATUS.ENABLED),
        schedulable: true,
      };
      const res = accountForm.id
        ? await API.put(`/api/account-pool/accounts/${accountForm.id}`, payload)
        : await API.post(
            `/api/account-pool/groups/${selectedGroupId}/accounts`,
            payload,
          );
      const { success, message } = res.data || {};
      if (!success) {
        showError(message || t('操作失败'));
        return;
      }
      showSuccess(t('操作成功完成！'));
      setAccountFormVisible(false);
      await refreshAll();
    } catch (error) {
      showError(error?.message || t('操作失败'));
    } finally {
      setActionLoading(false);
    }
  };

  const updateAccountStatus = async (account, action) => {
    setActionLoading(true);
    try {
      const payload =
        action === 'clear'
          ? { clear_cooldown: true, reason: '' }
          : action === 'enable'
            ? {
                status: ACCOUNT_STATUS.ENABLED,
                clear_cooldown: true,
                schedulable: true,
              }
            : {
                status: ACCOUNT_STATUS.MANUAL_DISABLED,
                schedulable: false,
              };
      const res = await API.post(
        `/api/account-pool/accounts/${account.id}/status`,
        payload,
      );
      const { success, message } = res.data || {};
      if (!success) {
        showError(message || t('操作失败'));
        return;
      }
      showSuccess(t('操作成功完成！'));
      await refreshAll();
    } catch (error) {
      showError(error?.message || t('操作失败'));
    } finally {
      setActionLoading(false);
    }
  };

  const deleteAccount = (account) => {
    Modal.confirm({
      title: t('删除账号'),
      content: t('确定要删除此账号吗？'),
      onOk: async () => {
        setActionLoading(true);
        try {
          const res = await API.delete(
            `/api/account-pool/accounts/${account.id}`,
          );
          const { success, message } = res.data || {};
          if (!success) {
            showError(message || t('操作失败'));
            return;
          }
          showSuccess(t('操作成功完成！'));
          await refreshAll();
        } catch (error) {
          showError(error?.message || t('操作失败'));
        } finally {
          setActionLoading(false);
        }
      },
    });
  };

  const submitBatch = async () => {
    if (!selectedGroupId || !batchCredentials.trim()) {
      showError(t('请填写账号凭证'));
      return;
    }
    setActionLoading(true);
    try {
      const res = await API.post(
        `/api/account-pool/groups/${selectedGroupId}/accounts/batch`,
        {
          credentials: batchCredentials,
          name_prefix: selectedGroup?.name || t('账号'),
          platform: selectedGroup?.platform || '',
          auth_type: selectedGroup?.auth_type || '',
          weight: 1,
          status: ACCOUNT_STATUS.ENABLED,
        },
      );
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('导入失败'));
        return;
      }
      showSuccess(
        t('已导入 {{created}} 个账号，跳过 {{skipped}} 个', {
          created: data?.created || 0,
          skipped: data?.skipped || 0,
        }),
      );
      setBatchCredentials('');
      setBatchVisible(false);
      await refreshAll();
    } catch (error) {
      showError(error?.message || t('导入失败'));
    } finally {
      setActionLoading(false);
    }
  };

  const startCodexOAuth = async () => {
    if (!selectedGroupId) return;
    setActionLoading(true);
    try {
      const res = await API.post('/api/account-pool/oauth/codex/start', {
        pool_group_id: selectedGroupId,
      });
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('操作失败'));
        return;
      }
      if (data?.authorize_url) {
        window.open(data.authorize_url, '_blank', 'noopener,noreferrer');
      }
      setOauthVisible(true);
    } catch (error) {
      showError(error?.message || t('操作失败'));
    } finally {
      setActionLoading(false);
    }
  };

  const completeCodexOAuth = async () => {
    if (!selectedGroupId || !oauthInput.trim()) {
      showError(t('请填写授权回调地址'));
      return;
    }
    setActionLoading(true);
    try {
      const res = await API.post('/api/account-pool/oauth/codex/complete', {
        pool_group_id: selectedGroupId,
        input: oauthInput.trim(),
        name: oauthName.trim(),
      });
      const { success, message } = res.data || {};
      if (!success) {
        showError(message || t('操作失败'));
        return;
      }
      showSuccess(t('操作成功完成！'));
      setOauthVisible(false);
      setOauthInput('');
      setOauthName('');
      await refreshAll();
    } catch (error) {
      showError(error?.message || t('操作失败'));
    } finally {
      setActionLoading(false);
    }
  };

  const refreshCredential = async (account) => {
    setActionLoading(true);
    try {
      const res = await API.post(
        `/api/account-pool/accounts/${account.id}/refresh`,
      );
      const { success, message } = res.data || {};
      if (!success) {
        showError(message || t('操作失败'));
        return;
      }
      showSuccess(t('操作成功完成！'));
      await refreshAll();
    } catch (error) {
      showError(error?.message || t('操作失败'));
    } finally {
      setActionLoading(false);
    }
  };

  const accountColumns = [
    {
      title: t('名称'),
      dataIndex: 'name',
      render: (value, account) => (
        <div>
          <div>{value || `#${account.id}`}</div>
          <Text type='tertiary' size='small'>
            #{account.id} · {account.platform}/{account.auth_type}
          </Text>
        </div>
      ),
    },
    {
      title: t('凭证'),
      dataIndex: 'credential_summary',
      render: (value) => <Text code>{formatCredentialSummary(value)}</Text>,
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      render: (_, account) => {
        const status = getAccountStatus(account, t);
        return (
          <Tag color={status.color} shape='circle'>
            {status.text}
          </Tag>
        );
      },
    },
    {
      title: t('模型'),
      dataIndex: 'models',
      render: (value) => value || t('继承账号组'),
    },
    {
      title: t('分组'),
      dataIndex: 'group',
      render: (value) => value || t('继承账号组'),
    },
    {
      title: t('优先级'),
      dataIndex: 'priority',
      width: 90,
    },
    {
      title: t('权重'),
      dataIndex: 'weight',
      width: 80,
      render: (value) => value || 1,
    },
    {
      title: t('冷却至'),
      dataIndex: 'rate_limited_until',
      render: (_, account) => formatUnixTime(getCoolingUntil(account)),
    },
    {
      title: t('操作'),
      dataIndex: 'operate',
      fixed: 'right',
      render: (_, account) => (
        <Space wrap>
          <Button
            size='small'
            type='tertiary'
            onClick={() => openEditAccount(account)}
          >
            {t('编辑')}
          </Button>
          {account.status === ACCOUNT_STATUS.ENABLED &&
          account.schedulable !== false ? (
            <Button
              size='small'
              type='danger'
              loading={actionLoading}
              onClick={() => updateAccountStatus(account, 'disable')}
            >
              {t('禁用')}
            </Button>
          ) : (
            <Button
              size='small'
              loading={actionLoading}
              onClick={() => updateAccountStatus(account, 'enable')}
            >
              {t('启用')}
            </Button>
          )}
          <Button
            size='small'
            type='tertiary'
            loading={actionLoading}
            onClick={() => updateAccountStatus(account, 'clear')}
          >
            {t('清除冷却')}
          </Button>
          {account.platform === 'codex' &&
            account.auth_type === 'official_oauth' && (
              <Button
                size='small'
                type='tertiary'
                loading={actionLoading}
                onClick={() => refreshCredential(account)}
              >
                {t('刷新凭证')}
              </Button>
            )}
          <Button
            size='small'
            type='danger'
            loading={actionLoading}
            onClick={() => deleteAccount(account)}
          >
            {t('删除')}
          </Button>
        </Space>
      ),
    },
  ];

  return (
    <div className='mt-[60px] px-2'>
      <Card>
        <div className='mb-4 flex flex-wrap items-center justify-between gap-3'>
          <div>
            <Title heading={4}>{t('账号池管理')}</Title>
            <Text type='tertiary'>
              {t('管理官方登录账号，并将账号组开放给渠道引用。')}
            </Text>
          </div>
          <Space wrap>
            <Button type='tertiary' onClick={refreshAll}>
              {t('刷新')}
            </Button>
            <Button type='primary' onClick={openCreateGroup}>
              {t('新建账号组')}
            </Button>
          </Space>
        </div>

        <div className='grid gap-4 lg:grid-cols-[320px_minmax(0,1fr)]'>
          <Card bodyStyle={{ padding: 0 }}>
            <div className='border-b p-3 font-medium'>{t('账号组')}</div>
            {groups.length === 0 ? (
              <div className='p-6 text-center text-gray-500'>
                {t('暂无账号组')}
              </div>
            ) : (
              groups.map((group) => (
                <button
                  key={group.id}
                  type='button'
                  className={`block w-full border-b p-3 text-left hover:bg-gray-50 ${
                    selectedGroupId === group.id ? 'bg-blue-50' : ''
                  }`}
                  onClick={() => {
                    setSelectedGroupId(group.id);
                    setPage(1);
                  }}
                >
                  <div className='flex items-start justify-between gap-2'>
                    <div>
                      <div className='font-medium'>{group.name}</div>
                      <Text type='tertiary' size='small'>
                        {group.platform}/{group.auth_type} · {group.strategy}
                      </Text>
                    </div>
                    <Tag
                      color={
                        group.status === ACCOUNT_STATUS.ENABLED
                          ? 'green'
                          : 'red'
                      }
                      shape='circle'
                    >
                      {group.status === ACCOUNT_STATUS.ENABLED
                        ? t('已启用')
                        : t('已禁用')}
                    </Tag>
                  </div>
                  <div className='mt-2 flex gap-3 text-xs text-gray-500'>
                    <span>
                      {t('总数')}: {group.stats?.total || 0}
                    </span>
                    <span>
                      {t('可用')}: {group.stats?.enabled || 0}
                    </span>
                    <span>
                      {t('冷却')}: {group.stats?.cooldown || 0}
                    </span>
                  </div>
                </button>
              ))
            )}
          </Card>

          <Card>
            <div className='mb-4 flex flex-wrap items-center justify-between gap-2'>
              <div>
                <div className='font-medium'>
                  {selectedGroup?.name || t('请选择账号组')}
                </div>
                {selectedGroup && (
                  <Text type='tertiary' size='small'>
                    {selectedGroup.models || t('全部模型')}
                  </Text>
                )}
              </div>
              {selectedGroup && (
                <Space wrap>
                  <Button
                    type='tertiary'
                    onClick={() => openEditGroup(selectedGroup)}
                  >
                    {t('编辑账号组')}
                  </Button>
                  <Button
                    type='danger'
                    onClick={() => deleteGroup(selectedGroup)}
                  >
                    {t('删除账号组')}
                  </Button>
                  <Button type='tertiary' onClick={startCodexOAuth}>
                    {t('Codex OAuth')}
                  </Button>
                  <Button type='tertiary' onClick={() => setBatchVisible(true)}>
                    {t('批量导入')}
                  </Button>
                  <Button type='primary' onClick={openCreateAccount}>
                    {t('添加账号')}
                  </Button>
                </Space>
              )}
            </div>

            <Space wrap className='mb-4'>
              <Tag color='grey' type='light'>
                {t('总数')}: {stats.total}
              </Tag>
              <Tag color='green' type='light'>
                {t('已启用')}: {stats.enabled}
              </Tag>
              <Tag color='orange' type='light'>
                {t('冷却中')}: {stats.cooldown}
              </Tag>
              <Tag color='red' type='light'>
                {t('已禁用')}: {stats.disabled}
              </Tag>
            </Space>

            <div className='mb-4 flex flex-wrap items-center gap-2'>
              <Input
                value={search}
                placeholder={t('搜索账号')}
                onChange={(value) => {
                  setSearch(value);
                  setPage(1);
                }}
                style={{ width: 220 }}
                showClear
              />
              <Select
                value={statusFilter}
                optionList={[
                  { label: t('全部'), value: 'all' },
                  { label: t('已启用'), value: '1' },
                  { label: t('已禁用'), value: '2' },
                  { label: t('自动禁用'), value: '3' },
                ]}
                onChange={(value) => {
                  setStatusFilter(value);
                  setPage(1);
                }}
                style={{ width: 140 }}
              />
            </div>

            <Table
              rowKey='id'
              loading={loading}
              columns={accountColumns}
              dataSource={accounts}
              pagination={{
                currentPage: page,
                pageSize,
                total,
                showSizeChanger: true,
                pageSizeOpts: [10, 20, 50],
                onPageChange: setPage,
                onPageSizeChange: (size) => {
                  setPageSize(size);
                  setPage(1);
                },
              }}
              scroll={{ x: 'max-content' }}
            />
          </Card>
        </div>
      </Card>

      <Modal
        title={groupForm.id ? t('编辑账号组') : t('新建账号组')}
        visible={groupFormVisible}
        onCancel={() => setGroupFormVisible(false)}
        onOk={submitGroup}
        confirmLoading={actionLoading}
        width={720}
      >
        <Form labelPosition='left'>
          <Form.Input
            label={t('名称')}
            field='name'
            value={groupForm.name}
            onChange={(value) => setGroupForm({ ...groupForm, name: value })}
          />
          <Form.Input
            label={t('平台')}
            field='platform'
            value={groupForm.platform}
            onChange={(value) =>
              setGroupForm({ ...groupForm, platform: value })
            }
          />
          <Form.Select
            label={t('认证类型')}
            field='auth_type'
            value={groupForm.auth_type}
            optionList={[
              { label: 'api_key', value: 'api_key' },
              { label: 'official_oauth', value: 'official_oauth' },
              { label: 'cookie', value: 'cookie' },
              { label: 'service_account', value: 'service_account' },
              { label: 'custom_json', value: 'custom_json' },
            ]}
            onChange={(value) =>
              setGroupForm({ ...groupForm, auth_type: value })
            }
          />
          <Form.Select
            label={t('轮询策略')}
            field='strategy'
            value={groupForm.strategy}
            optionList={[
              { label: 'round_robin', value: 'round_robin' },
              { label: 'weighted', value: 'weighted' },
              { label: 'fill_first', value: 'fill_first' },
              { label: 'least_used', value: 'least_used' },
            ]}
            onChange={(value) =>
              setGroupForm({ ...groupForm, strategy: value })
            }
          />
          <Form.Input
            label={t('模型')}
            field='models'
            value={groupForm.models}
            placeholder={t('留空表示全部模型')}
            onChange={(value) => setGroupForm({ ...groupForm, models: value })}
          />
          <Form.Input
            label={t('分组')}
            field='group'
            value={groupForm.group}
            placeholder={t('留空表示全部分组')}
            onChange={(value) => setGroupForm({ ...groupForm, group: value })}
          />
        </Form>
      </Modal>

      <Modal
        title={accountForm.id ? t('编辑账号') : t('添加账号')}
        visible={accountFormVisible}
        onCancel={() => setAccountFormVisible(false)}
        onOk={submitAccount}
        confirmLoading={actionLoading}
        width={720}
      >
        <Form labelPosition='left'>
          <Form.Input
            label={t('名称')}
            field='name'
            value={accountForm.name}
            onChange={(value) =>
              setAccountForm({ ...accountForm, name: value })
            }
          />
          <Form.Input
            label={t('凭证')}
            field='credentials'
            value={accountForm.credentials}
            placeholder={
              accountForm.id ? t('留空则保留现有凭证') : t('请输入账号凭证')
            }
            onChange={(value) =>
              setAccountForm({ ...accountForm, credentials: value })
            }
          />
          <Form.Input
            label={t('平台')}
            field='platform'
            value={accountForm.platform}
            onChange={(value) =>
              setAccountForm({ ...accountForm, platform: value })
            }
          />
          <Form.Select
            label={t('认证类型')}
            field='auth_type'
            value={accountForm.auth_type}
            optionList={[
              { label: 'api_key', value: 'api_key' },
              { label: 'official_oauth', value: 'official_oauth' },
              { label: 'cookie', value: 'cookie' },
              { label: 'service_account', value: 'service_account' },
              { label: 'custom_json', value: 'custom_json' },
            ]}
            onChange={(value) =>
              setAccountForm({ ...accountForm, auth_type: value })
            }
          />
          <Form.Input
            label={t('模型')}
            field='models'
            value={accountForm.models}
            placeholder={t('留空则继承账号组')}
            onChange={(value) =>
              setAccountForm({ ...accountForm, models: value })
            }
          />
          <Form.Input
            label={t('分组')}
            field='group'
            value={accountForm.group}
            placeholder={t('留空则继承账号组')}
            onChange={(value) =>
              setAccountForm({ ...accountForm, group: value })
            }
          />
          <Form.InputNumber
            label={t('优先级')}
            field='priority'
            value={accountForm.priority}
            onNumberChange={(value) =>
              setAccountForm({ ...accountForm, priority: value || 0 })
            }
            style={{ width: '100%' }}
          />
          <Form.InputNumber
            label={t('权重')}
            field='weight'
            min={0}
            value={accountForm.weight}
            onNumberChange={(value) =>
              setAccountForm({ ...accountForm, weight: value || 1 })
            }
            style={{ width: '100%' }}
          />
          <Form.InputNumber
            label={t('最大并发')}
            field='max_concurrency'
            min={0}
            value={accountForm.max_concurrency}
            onNumberChange={(value) =>
              setAccountForm({ ...accountForm, max_concurrency: value || 0 })
            }
            style={{ width: '100%' }}
          />
          <Form.Input
            label={t('代理')}
            field='proxy'
            value={accountForm.proxy}
            onChange={(value) =>
              setAccountForm({ ...accountForm, proxy: value })
            }
          />
        </Form>
      </Modal>

      <Modal
        title={t('批量导入账号')}
        visible={batchVisible}
        onCancel={() => setBatchVisible(false)}
        onOk={submitBatch}
        confirmLoading={actionLoading}
      >
        <TextArea
          value={batchCredentials}
          placeholder={t('请输入账号凭证，一行一个')}
          autosize={{ minRows: 6, maxRows: 12 }}
          onChange={setBatchCredentials}
        />
      </Modal>

      <Modal
        title={t('完成 Codex OAuth')}
        visible={oauthVisible}
        onCancel={() => setOauthVisible(false)}
        onOk={completeCodexOAuth}
        confirmLoading={actionLoading}
      >
        <Form labelPosition='left'>
          <Form.Input
            label={t('账号名称')}
            field='oauth_name'
            value={oauthName}
            placeholder={t('可选')}
            onChange={setOauthName}
          />
          <Form.TextArea
            label={t('授权回调地址')}
            field='oauth_input'
            value={oauthInput}
            placeholder={t('粘贴 Codex 授权完成后的回调地址')}
            autosize={{ minRows: 4, maxRows: 8 }}
            onChange={setOauthInput}
          />
        </Form>
      </Modal>
    </div>
  );
};

export default AccountPool;
