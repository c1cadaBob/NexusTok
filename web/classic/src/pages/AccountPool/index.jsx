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

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Card, Space, Table, Tag, Typography } from '@douyinfe/semi-ui';
import { IconRefresh } from '@douyinfe/semi-icons';
import { API, showError } from '../../helpers';

const { Text, Title } = Typography;

const PAGE_SIZE = 20;

const normalizeStats = (stats = {}) => ({
  total: Number(stats.total || 0),
  enabled: Number(stats.enabled || 0),
  disabled: Number(stats.disabled || 0),
  cooldown: Number(stats.cooldown || 0),
});

const formatTime = (value) => {
  const timestamp = Number(value || 0);
  if (!timestamp) return '-';
  return new Date(timestamp * 1000).toLocaleString();
};

const AccountPool = () => {
  const { t } = useTranslation();
  const [groups, setGroups] = useState([]);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);

  const fetchGroups = useCallback(
    async (nextPage = page) => {
      setLoading(true);
      try {
        const res = await API.get('/api/account-pool/groups', {
          disableDuplicate: true,
          params: { p: nextPage, page_size: PAGE_SIZE },
          skipErrorHandler: true,
        });
        const payload = res.data?.data || {};
        setGroups(Array.isArray(payload.items) ? payload.items : []);
        setPage(Number(payload.page || nextPage));
        setTotal(Number(payload.total || 0));
      } catch (error) {
        showError(error);
        setGroups([]);
        setTotal(0);
      } finally {
        setLoading(false);
      }
    },
    [page],
  );

  useEffect(() => {
    fetchGroups(1);
    // 账号池页面首次进入时只加载第一页；翻页由 Table 的 onPageChange 主动触发。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const summary = useMemo(
    () =>
      groups.reduce(
        (acc, group) => {
          const stats = normalizeStats(group?.stats);
          acc.total += stats.total;
          acc.enabled += stats.enabled;
          acc.disabled += stats.disabled;
          acc.cooldown += stats.cooldown;
          return acc;
        },
        { total: 0, enabled: 0, disabled: 0, cooldown: 0 },
      ),
    [groups],
  );

  const columns = useMemo(
    () => [
      {
        title: t('名称'),
        dataIndex: 'name',
        render: (name, record) => (
          <Space vertical spacing={2} align='start'>
            <Text strong>{name || '-'}</Text>
            <Space spacing={4}>
              <Tag color='green'>native</Tag>
              <Text type='tertiary' size='small'>
                {record.platform || '-'} / {record.auth_type || '-'}
              </Text>
            </Space>
          </Space>
        ),
      },
      {
        title: t('账号池策略'),
        dataIndex: 'strategy',
        render: (strategy) => strategy || '-',
      },
      {
        title: t('模型'),
        dataIndex: 'models',
        render: (models) => models || '-',
      },
      {
        title: t('状态'),
        dataIndex: 'status',
        render: (status) =>
          Number(status) === 1 ? (
            <Tag color='green'>{t('启用')}</Tag>
          ) : (
            <Tag color='red'>{t('禁用')}</Tag>
          ),
      },
      {
        title: t('总数'),
        dataIndex: 'stats',
        render: (stats) => {
          const normalized = normalizeStats(stats);
          return (
            <Space spacing={6} wrap>
              <Tag>{normalized.total}</Tag>
              <Tag color='green'>
                {t('启用')} {normalized.enabled}
              </Tag>
              <Tag color='red'>
                {t('禁用')} {normalized.disabled}
              </Tag>
              <Tag color='orange'>
                {t('冷却中')} {normalized.cooldown}
              </Tag>
            </Space>
          );
        },
      },
      {
        title: t('更新时间'),
        dataIndex: 'updated_time',
        render: formatTime,
      },
    ],
    [t],
  );

  return (
    <div style={{ padding: '32px 24px 40px' }}>
      <Card
        bordered={false}
        bodyStyle={{ padding: 24 }}
        headerStyle={{ padding: '22px 24px 0' }}
        title={
          <Space vertical spacing={8} align='start'>
            <Title heading={4} style={{ margin: 0 }}>
              {t('账号池管理')}
            </Title>
            <Space spacing={8} wrap>
              <Tag color='blue'>
                {t('总数')} {summary.total}
              </Tag>
              <Tag color='green'>
                {t('启用')} {summary.enabled}
              </Tag>
              <Tag color='red'>
                {t('禁用')} {summary.disabled}
              </Tag>
              <Tag color='orange'>
                {t('冷却中')} {summary.cooldown}
              </Tag>
            </Space>
          </Space>
        }
        headerExtraContent={
          <Button
            icon={<IconRefresh />}
            loading={loading}
            onClick={() => fetchGroups(page)}
            theme='borderless'
          >
            {t('刷新')}
          </Button>
        }
      >
        <Table
          columns={columns}
          dataSource={groups}
          empty={t('暂无数据')}
          loading={loading}
          pagination={{
            currentPage: page,
            pageSize: PAGE_SIZE,
            total,
            showSizeChanger: false,
            onPageChange: (nextPage) => fetchGroups(nextPage),
          }}
          rowKey='id'
        />
      </Card>
    </div>
  );
};

export default AccountPool;
