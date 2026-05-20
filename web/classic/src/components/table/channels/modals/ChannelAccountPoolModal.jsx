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
  Form,
  Input,
  Modal,
  Select,
  Space,
  Table,
  Tag,
  TextArea,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import { API, showError, showSuccess } from '../../../../helpers';

const { Text } = Typography;

const CHANNEL_STATUS = {
  ENABLED: 1,
  MANUAL_DISABLED: 2,
  AUTO_DISABLED: 3,
};

const emptyForm = {
  id: undefined,
  name: '',
  key: '',
  models: '',
  group: '',
  priority: 0,
  weight: 1,
  max_concurrency: 0,
};

const defaultStats = {
  total: 0,
  enabled: 0,
  disabled: 0,
  cooldown: 0,
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
  const coolingUntil = getCoolingUntil(account);
  if (account.status !== CHANNEL_STATUS.ENABLED) {
    return {
      color: account.status === CHANNEL_STATUS.AUTO_DISABLED ? 'yellow' : 'red',
      text:
        account.status === CHANNEL_STATUS.AUTO_DISABLED
          ? t('自动禁用')
          : t('已禁用'),
    };
  }
  if (coolingUntil > now) {
    return { color: 'orange', text: t('冷却中') };
  }
  return { color: 'green', text: t('已启用') };
}

export default function ChannelAccountPoolModal({
  visible,
  onCancel,
  channel,
  onRefresh,
}) {
  const { t } = useTranslation();
  const channelId = channel?.id;
  const [loading, setLoading] = useState(false);
  const [accounts, setAccounts] = useState([]);
  const [stats, setStats] = useState(defaultStats);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState('all');
  const [formVisible, setFormVisible] = useState(false);
  const [formState, setFormState] = useState(emptyForm);
  const [batchVisible, setBatchVisible] = useState(false);
  const [batchKeys, setBatchKeys] = useState('');
  const [actionLoading, setActionLoading] = useState(false);

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

  const loadAccounts = async () => {
    if (!visible || !channelId) return;
    setLoading(true);
    try {
      const res = await API.get(`/api/channel/${channelId}/accounts`, {
        params: queryParams,
      });
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
    loadAccounts();
  }, [visible, channelId, queryParams]);

  const refreshAll = async () => {
    await loadAccounts();
    onRefresh?.();
  };

  const openCreate = () => {
    setFormState({
      ...emptyForm,
      models: channel?.models || '',
      group: channel?.group || '',
      priority: channel?.priority || 0,
      weight: channel?.weight || 1,
    });
    setFormVisible(true);
  };

  const openEdit = (account) => {
    setFormState({
      id: account.id,
      name: account.name || '',
      key: '',
      models: account.models || '',
      group: account.group || '',
      priority: account.priority || 0,
      weight: account.weight || 1,
      max_concurrency: account.max_concurrency || 0,
    });
    setFormVisible(true);
  };

  const submitForm = async () => {
    if (!channelId) return;
    if (!formState.id && !formState.key.trim()) {
      showError(t('请填写账号密钥'));
      return;
    }
    setActionLoading(true);
    try {
      const payload = {
        name: formState.name.trim(),
        key: formState.key.trim(),
        models: formState.models.trim(),
        group: formState.group.trim(),
        priority: Number(formState.priority || 0),
        weight: Number(formState.weight || 1),
        max_concurrency: Number(formState.max_concurrency || 0),
        status: CHANNEL_STATUS.ENABLED,
      };
      const res = formState.id
        ? await API.put(
            `/api/channel/${channelId}/accounts/${formState.id}`,
            payload,
          )
        : await API.post(`/api/channel/${channelId}/accounts`, payload);
      const { success, message } = res.data || {};
      if (!success) {
        showError(message || t('操作失败'));
        return;
      }
      showSuccess(formState.id ? t('账号已更新') : t('账号已创建'));
      setFormVisible(false);
      await refreshAll();
    } catch (error) {
      showError(error?.message || t('操作失败'));
    } finally {
      setActionLoading(false);
    }
  };

  const updateStatus = async (account, action) => {
    if (!channelId) return;
    setActionLoading(true);
    try {
      const payload =
        action === 'clear'
          ? { clear_cooldown: true, reason: '' }
          : action === 'enable'
            ? {
                status: CHANNEL_STATUS.ENABLED,
                clear_cooldown: true,
                reason: '',
              }
            : {
                status: CHANNEL_STATUS.MANUAL_DISABLED,
                clear_cooldown: false,
                reason: '',
              };
      const res = await API.post(
        `/api/channel/${channelId}/accounts/${account.id}/status`,
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

  const deleteAccount = async (account) => {
    if (!channelId) return;
    Modal.confirm({
      title: t('删除账号'),
      content: t('确定要删除此账号吗？'),
      onOk: async () => {
        setActionLoading(true);
        try {
          const res = await API.delete(
            `/api/channel/${channelId}/accounts/${account.id}`,
          );
          const { success, message } = res.data || {};
          if (!success) {
            showError(message || t('操作失败'));
            return;
          }
          showSuccess(t('账号已删除'));
          await refreshAll();
        } catch (error) {
          showError(error?.message || t('操作失败'));
        } finally {
          setActionLoading(false);
        }
      },
    });
  };

  const submitBatch = async (fromMultiKey = false) => {
    if (!channelId) return;
    if (!fromMultiKey && !batchKeys.trim()) {
      showError(t('请填写账号密钥'));
      return;
    }
    setActionLoading(true);
    try {
      const payload = {
        keys: batchKeys,
        name_prefix: channel?.name || '',
        models: channel?.models || '',
        group: channel?.group || '',
        priority: channel?.priority || 0,
        weight: channel?.weight || 1,
        status: CHANNEL_STATUS.ENABLED,
      };
      const res = fromMultiKey
        ? await API.post(
            `/api/channel/${channelId}/accounts/import-multikey`,
            payload,
          )
        : await API.post(`/api/channel/${channelId}/accounts/batch`, payload);
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
      setBatchKeys('');
      setBatchVisible(false);
      await refreshAll();
    } catch (error) {
      showError(error?.message || t('导入失败'));
    } finally {
      setActionLoading(false);
    }
  };

  const columns = [
    {
      title: t('名称'),
      dataIndex: 'name',
      render: (value, account) => value || `#${account.id}`,
    },
    {
      title: t('密钥'),
      dataIndex: 'key',
      render: (value) => <Text code>{value || '-'}</Text>,
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      render: (_, account) => {
        const status = getAccountStatus(account, t);
        return (
          <Tooltip
            content={account.disabled_reason || account.last_error || ''}
          >
            <Tag color={status.color} shape='circle'>
              {status.text}
            </Tag>
          </Tooltip>
        );
      },
    },
    {
      title: t('模型'),
      dataIndex: 'models',
      render: (value) => value || t('继承渠道'),
    },
    {
      title: t('分组'),
      dataIndex: 'group',
      render: (value) => value || t('继承渠道'),
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
            onClick={() => openEdit(account)}
          >
            {t('编辑')}
          </Button>
          {account.status === CHANNEL_STATUS.ENABLED ? (
            <Button
              size='small'
              type='danger'
              loading={actionLoading}
              onClick={() => updateStatus(account, 'disable')}
            >
              {t('禁用')}
            </Button>
          ) : (
            <Button
              size='small'
              loading={actionLoading}
              onClick={() => updateStatus(account, 'enable')}
            >
              {t('启用')}
            </Button>
          )}
          <Button
            size='small'
            type='tertiary'
            loading={actionLoading}
            onClick={() => updateStatus(account, 'clear')}
          >
            {t('清除冷却')}
          </Button>
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
    <>
      <Modal
        title={`${t('账号池')} - ${channel?.name || ''}`}
        visible={visible}
        onCancel={onCancel}
        width={1100}
        footer={null}
        closeOnEsc
      >
        <div className='space-y-4'>
          <Space wrap>
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

          <div className='flex flex-wrap items-center justify-between gap-2'>
            <Space wrap>
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
            </Space>
            <Space wrap>
              <Button type='tertiary' onClick={refreshAll}>
                {t('刷新')}
              </Button>
              <Button type='tertiary' onClick={() => setBatchVisible(true)}>
                {t('批量导入')}
              </Button>
              <Button type='primary' onClick={openCreate}>
                {t('添加账号')}
              </Button>
            </Space>
          </div>

          <Table
            rowKey='id'
            loading={loading}
            columns={columns}
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
        </div>
      </Modal>

      <Modal
        title={formState.id ? t('编辑账号') : t('添加账号')}
        visible={formVisible}
        onCancel={() => setFormVisible(false)}
        onOk={submitForm}
        confirmLoading={actionLoading}
      >
        <Form labelPosition='left'>
          <Form.Input
            label={t('名称')}
            field='name'
            value={formState.name}
            onChange={(value) => setFormState({ ...formState, name: value })}
            showClear
          />
          <Form.Input
            label={t('密钥')}
            field='key'
            value={formState.key}
            placeholder={
              formState.id ? t('留空则保留现有密钥') : t('请输入密钥')
            }
            onChange={(value) => setFormState({ ...formState, key: value })}
            showClear
          />
          <Form.Input
            label={t('模型')}
            field='models'
            value={formState.models}
            placeholder={t('留空则继承渠道模型')}
            onChange={(value) => setFormState({ ...formState, models: value })}
            showClear
          />
          <Form.Input
            label={t('分组')}
            field='group'
            value={formState.group}
            placeholder={t('留空则继承渠道分组')}
            onChange={(value) => setFormState({ ...formState, group: value })}
            showClear
          />
          <Form.InputNumber
            label={t('优先级')}
            field='priority'
            value={formState.priority}
            onNumberChange={(value) =>
              setFormState({ ...formState, priority: value || 0 })
            }
            style={{ width: '100%' }}
          />
          <Form.InputNumber
            label={t('权重')}
            field='weight'
            min={0}
            value={formState.weight}
            onNumberChange={(value) =>
              setFormState({ ...formState, weight: value || 1 })
            }
            style={{ width: '100%' }}
          />
          <Form.InputNumber
            label={t('最大并发')}
            field='max_concurrency'
            min={0}
            value={formState.max_concurrency}
            onNumberChange={(value) =>
              setFormState({ ...formState, max_concurrency: value || 0 })
            }
            style={{ width: '100%' }}
          />
        </Form>
      </Modal>

      <Modal
        title={t('批量导入账号')}
        visible={batchVisible}
        onCancel={() => setBatchVisible(false)}
        footer={
          <Space>
            <Button onClick={() => setBatchVisible(false)}>{t('取消')}</Button>
            <Button
              type='tertiary'
              loading={actionLoading}
              onClick={() => submitBatch(true)}
            >
              {t('从多密钥导入')}
            </Button>
            <Button
              type='primary'
              loading={actionLoading}
              onClick={() => submitBatch(false)}
            >
              {t('导入密钥')}
            </Button>
          </Space>
        }
      >
        <TextArea
          value={batchKeys}
          placeholder={t('请输入账号密钥，一行一个')}
          autosize={{ minRows: 6, maxRows: 12 }}
          onChange={setBatchKeys}
        />
        <Text type='tertiary' size='small' className='mt-2 block'>
          {t('从多密钥导入不会删除原渠道密钥，也不会自动切换凭证模式。')}
        </Text>
      </Modal>
    </>
  );
}
