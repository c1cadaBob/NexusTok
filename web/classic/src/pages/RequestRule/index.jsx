import React, { useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Table,
  Button,
  Modal,
  Form,
  Switch,
  Select,
  Tag,
  Tabs,
  TabPane,
  Popconfirm,
  Typography,
  Descriptions,
} from '@douyinfe/semi-ui';
import { IconPlus, IconDelete } from '@douyinfe/semi-icons';
import { API, showError, showSuccess } from '../../helpers';

// 中转格式选项
const RELAY_FORMAT_OPTIONS = [
  { value: '', label: 'All' },
  { value: 'openai', label: 'OpenAI' },
  { value: 'claude', label: 'Claude' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'openai_responses', label: 'OpenAI Responses' },
  { value: 'openai_image', label: 'OpenAI Image' },
  { value: 'embedding', label: 'Embedding' },
  { value: 'rerank', label: 'Rerank' },
];

// 模型匹配模式选项
const MODEL_MATCH_MODE_OPTIONS = [
  { value: 0, label: 'Exact Match' },
  { value: 1, label: 'Prefix Match' },
  { value: 2, label: 'Contains Match' },
  { value: 3, label: 'Suffix Match' },
  { value: 4, label: 'Wildcard Match' },
];

// 默认规则表单值
const defaultRuleValues = {
  name: '',
  description: '',
  status: 1,
  priority: 0,
  relay_format: '',
  model_pattern: '',
  model_match_mode: 0,
  param_override: '',
  header_override: '',
  log_request: false,
  log_response: false,
  log_max_size: 4096,
};

const RequestRule = () => {
  const { t } = useTranslation();

  // ========== 规则列表状态 ==========
  const [rules, setRules] = useState([]);
  const [rulesTotal, setRulesTotal] = useState(0);
  const [rulesPage, setRulesPage] = useState(1);
  const [rulesPageSize, setRulesPageSize] = useState(20);
  const [rulesLoading, setRulesLoading] = useState(false);

  // ========== 规则表单 Modal ==========
  const [ruleModalVisible, setRuleModalVisible] = useState(false);
  const [editingRule, setEditingRule] = useState(null);
  const [ruleFormLoading, setRuleFormLoading] = useState(false);
  const [ruleFormApi, setRuleFormApi] = useState(null);

  // ========== 请求记录状态 ==========
  const [logs, setLogs] = useState([]);
  const [logsTotal, setLogsTotal] = useState(0);
  const [logsPage, setLogsPage] = useState(1);
  const [logsPageSize, setLogsPageSize] = useState(20);
  const [logsLoading, setLogsLoading] = useState(false);

  // ========== 日志详情 Modal ==========
  const [logDetailVisible, setLogDetailVisible] = useState(false);
  const [logDetail, setLogDetail] = useState(null);
  const [logDetailLoading, setLogDetailLoading] = useState(false);

  // ========== 规则 CRUD ==========
  const loadRules = useCallback(async () => {
    setRulesLoading(true);
    try {
      const res = await API.get('/api/request_rule/', {
        params: { p: rulesPage, page_size: rulesPageSize },
      });
      const { success, data, message } = res.data;
      if (success) {
        setRules(data?.items || []);
        setRulesTotal(data?.total || 0);
      } else {
        showError(message);
      }
    } catch (e) {
      showError(e.message);
    }
    setRulesLoading(false);
  }, [rulesPage, rulesPageSize]);

  useEffect(() => {
    loadRules();
  }, [loadRules]);

  const handleCreateRule = () => {
    setEditingRule(null);
    setRuleModalVisible(true);
  };

  const handleEditRule = (rule) => {
    setEditingRule(rule);
    setRuleModalVisible(true);
  };

  const handleDeleteRule = async (id) => {
    try {
      const res = await API.delete(`/api/request_rule/${id}`);
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('删除成功'));
        loadRules();
      } else {
        showError(message);
      }
    } catch (e) {
      showError(e.message);
    }
  };

  const handleToggleRuleStatus = async (id, currentStatus) => {
    const newStatus = currentStatus === 1 ? 0 : 1;
    try {
      const res = await API.put(`/api/request_rule/${id}/status`, {
        status: newStatus,
      });
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('状态更新成功'));
        loadRules();
      } else {
        showError(message);
      }
    } catch (e) {
      showError(e.message);
    }
  };

  const handleRuleSubmit = async () => {
    if (!ruleFormApi) return;
    try {
      const values = await ruleFormApi.validate();
      setRuleFormLoading(true);
      // 处理空字符串 JSON 字段转为 null
      const payload = {
        ...values,
        param_override: values.param_override || null,
        header_override: values.header_override || null,
      };

      let res;
      if (editingRule) {
        res = await API.put('/api/request_rule/', {
          id: editingRule.id,
          ...payload,
        });
      } else {
        res = await API.post('/api/request_rule/', payload);
      }
      const { success, message } = res.data;
      if (success) {
        showSuccess(editingRule ? t('更新成功') : t('创建成功'));
        setRuleModalVisible(false);
        loadRules();
      } else {
        showError(message);
      }
    } catch (e) {
      // 表单校验失败会抛异常，不额外处理
      if (e?.message) showError(e.message);
    }
    setRuleFormLoading(false);
  };

  // ========== 请求记录 ==========
  const loadLogs = useCallback(async () => {
    setLogsLoading(true);
    try {
      const res = await API.get('/api/request_log/', {
        params: { p: logsPage, page_size: logsPageSize },
      });
      const { success, data, message } = res.data;
      if (success) {
        setLogs(data?.items || []);
        setLogsTotal(data?.total || 0);
      } else {
        showError(message);
      }
    } catch (e) {
      showError(e.message);
    }
    setLogsLoading(false);
  }, [logsPage, logsPageSize]);

  useEffect(() => {
    loadLogs();
  }, [loadLogs]);

  const handleViewLogDetail = async (log) => {
    setLogDetail(null);
    setLogDetailVisible(true);
    setLogDetailLoading(true);
    try {
      const res = await API.get(`/api/request_log/${log.id}`);
      const { success, data, message } = res.data;
      if (success) {
        setLogDetail(data);
      } else {
        showError(message);
      }
    } catch (e) {
      showError(e.message);
    }
    setLogDetailLoading(false);
  };

  const handleCleanupLogs = async () => {
    try {
      const res = await API.delete('/api/request_log/');
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('清理成功'));
        loadLogs();
      } else {
        showError(message);
      }
    } catch (e) {
      showError(e.message);
    }
  };

  // ========== 辅助函数 ==========
  const getRelayFormatLabel = (value) => {
    const opt = RELAY_FORMAT_OPTIONS.find((o) => o.value === value);
    return opt ? t(opt.label) : value || t('All');
  };

  const getMatchModeLabel = (value) => {
    const opt = MODEL_MATCH_MODE_OPTIONS.find((o) => o.value === value);
    return opt ? t(opt.label) : String(value);
  };

  const formatTime = (ts) => {
    if (!ts) return '-';
    return new Date(ts * 1000).toLocaleString();
  };

  const formatLatency = (ms) => {
    if (!ms && ms !== 0) return '-';
    if (ms < 1000) return `${ms}ms`;
    return `${(ms / 1000).toFixed(2)}s`;
  };

  // ========== 规则列表列定义 ==========
  const ruleColumns = [
    {
      title: t('名称'),
      dataIndex: 'name',
      key: 'name',
      render: (text, record) => (
        <div>
          <div style={{ fontWeight: 500 }}>{text}</div>
          {record.description && (
            <Typography.Text type='tertiary' size='small'>
              {record.description}
            </Typography.Text>
          )}
        </div>
      ),
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      key: 'status',
      width: 80,
      render: (_, record) => (
        <Switch
          size='small'
          checked={record.status === 1}
          onChange={() => handleToggleRuleStatus(record.id, record.status)}
        />
      ),
    },
    {
      title: t('优先级'),
      dataIndex: 'priority',
      key: 'priority',
      width: 80,
      render: (text) => <Tag size='small'>{text}</Tag>,
    },
    {
      title: t('中转格式'),
      dataIndex: 'relay_format',
      key: 'relay_format',
      width: 140,
      render: (text) => (
        <Tag size='small' color='blue'>
          {getRelayFormatLabel(text)}
        </Tag>
      ),
    },
    {
      title: t('模型匹配'),
      dataIndex: 'model_pattern',
      key: 'model_pattern',
      render: (text, record) => (
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <Typography.Text code size='small'>
            {text || '*'}
          </Typography.Text>
          <Tag size='small' color='light-blue'>
            {getMatchModeLabel(record.model_match_mode)}
          </Tag>
        </div>
      ),
    },
    {
      title: t('操作'),
      key: 'actions',
      width: 140,
      render: (_, record) => (
        <div style={{ display: 'flex', gap: 4 }}>
          <Button
            theme='light'
            type='primary'
            size='small'
            onClick={() => handleEditRule(record)}
          >
            {t('编辑')}
          </Button>
          <Popconfirm
            title={t('确认删除')}
            content={t('删除后无法恢复，确定要删除吗？')}
            onConfirm={() => handleDeleteRule(record.id)}
          >
            <Button theme='light' type='danger' size='small'>
              {t('删除')}
            </Button>
          </Popconfirm>
        </div>
      ),
    },
  ];

  // ========== 请求记录列定义 ==========
  const logColumns = [
    {
      title: 'ID',
      dataIndex: 'id',
      key: 'id',
      width: 70,
    },
    {
      title: t('规则 ID'),
      dataIndex: 'request_rule_id',
      key: 'request_rule_id',
      width: 90,
      render: (text) => <Tag size='small'>{text}</Tag>,
    },
    {
      title: t('模型'),
      dataIndex: 'model_name',
      key: 'model_name',
      render: (text) => (
        <Typography.Text code size='small'>
          {text}
        </Typography.Text>
      ),
    },
    {
      title: t('中转格式'),
      dataIndex: 'relay_format',
      key: 'relay_format',
      width: 120,
      render: (text) => (
        <Tag size='small' color='blue'>
          {text || '-'}
        </Tag>
      ),
    },
    {
      title: t('状态码'),
      dataIndex: 'status_code',
      key: 'status_code',
      width: 90,
      render: (code) => {
        let color = 'green';
        if (code >= 400) color = 'red';
        else if (code >= 300) color = 'orange';
        return (
          <Tag size='small' color={color}>
            {code}
          </Tag>
        );
      },
    },
    {
      title: t('延迟'),
      dataIndex: 'latency',
      key: 'latency',
      width: 100,
      render: (ms) => formatLatency(ms),
    },
    {
      title: t('时间'),
      dataIndex: 'created_at',
      key: 'created_at',
      width: 180,
      render: (ts) => (
        <Typography.Text type='tertiary' size='small'>
          {formatTime(ts)}
        </Typography.Text>
      ),
    },
    {
      title: t('操作'),
      key: 'actions',
      width: 80,
      render: (_, record) => (
        <Button
          theme='light'
          type='primary'
          size='small'
          onClick={() => handleViewLogDetail(record)}
        >
          {t('详情')}
        </Button>
      ),
    },
  ];

  return (
    <div className='mt-[60px] px-2'>
      <Tabs type='line' defaultActiveKey='rules'>
        {/* 规则列表 Tab */}
        <TabPane tab={t('规则列表')} itemKey='rules'>
          <div style={{ marginBottom: 16 }}>
            <Button
              theme='solid'
              type='primary'
              icon={<IconPlus />}
              onClick={handleCreateRule}
            >
              {t('创建规则')}
            </Button>
          </div>
          <Table
            columns={ruleColumns}
            dataSource={rules}
            rowKey='id'
            loading={rulesLoading}
            pagination={{
              currentPage: rulesPage,
              pageSize: rulesPageSize,
              total: rulesTotal,
              onPageChange: (page) => setRulesPage(page),
              onPageSizeChange: (size) => {
                setRulesPageSize(size);
                setRulesPage(1);
              },
              showSizeChanger: true,
              pageSizeOpts: [10, 20, 50],
            }}
          />
        </TabPane>

        {/* 请求记录 Tab */}
        <TabPane tab={t('请求记录')} itemKey='logs'>
          <div style={{ marginBottom: 16 }}>
            <Popconfirm
              title={t('确认清理')}
              content={t('将删除所有请求记录，确定要清理吗？')}
              onConfirm={handleCleanupLogs}
            >
              <Button
                theme='solid'
                type='danger'
                icon={<IconDelete />}
                disabled={logs.length === 0}
              >
                {t('清理记录')}
              </Button>
            </Popconfirm>
          </div>
          <Table
            columns={logColumns}
            dataSource={logs}
            rowKey='id'
            loading={logsLoading}
            pagination={{
              currentPage: logsPage,
              pageSize: logsPageSize,
              total: logsTotal,
              onPageChange: (page) => setLogsPage(page),
              onPageSizeChange: (size) => {
                setLogsPageSize(size);
                setLogsPage(1);
              },
              showSizeChanger: true,
              pageSizeOpts: [10, 20, 50],
            }}
          />
        </TabPane>
      </Tabs>

      {/* 规则创建/编辑 Modal */}
      <Modal
        title={editingRule ? t('编辑规则') : t('创建规则')}
        visible={ruleModalVisible}
        onOk={handleRuleSubmit}
        onCancel={() => setRuleModalVisible(false)}
        confirmLoading={ruleFormLoading}
        okText={editingRule ? t('更新') : t('创建')}
        cancelText={t('取消')}
        style={{ maxWidth: 640 }}
        bodyStyle={{ maxHeight: '65vh', overflow: 'auto' }}
      >
        <Form
          getFormApi={(api) => setRuleFormApi(api)}
          initValues={editingRule ? {
            name: editingRule.name,
            description: editingRule.description || '',
            status: editingRule.status,
            priority: editingRule.priority,
            relay_format: editingRule.relay_format || '',
            model_pattern: editingRule.model_pattern || '',
            model_match_mode: editingRule.model_match_mode,
            param_override: editingRule.param_override || '',
            header_override: editingRule.header_override || '',
            log_request: editingRule.log_request,
            log_response: editingRule.log_response,
            log_max_size: editingRule.log_max_size,
          } : defaultRuleValues}
          key={editingRule ? `edit-${editingRule.id}` : 'create'}
          labelPosition='top'
        >
          <Form.Input
            field='name'
            label={t('名称')}
            placeholder={t('规则名称')}
            rules={[{ required: true, message: t('请输入规则名称') }]}
          />
          <Form.Input
            field='description'
            label={t('描述')}
            placeholder={t('可选描述')}
          />
          <div style={{ display: 'flex', gap: 16 }}>
            <div style={{ flex: 1 }}>
              <Form.InputNumber
                field='priority'
                label={t('优先级')}
                placeholder='0'
                extraText={t('数值越大优先级越高')}
              />
            </div>
            <div style={{ flex: 1 }}>
              <Form.Switch
                field='status'
                label={t('启用')}
                checkedText={t('启用')}
                uncheckedText={t('禁用')}
                initValue={editingRule ? editingRule.status === 1 : true}
                onChange={(checked, e) => {
                  ruleFormApi?.setValue('status', checked ? 1 : 0);
                }}
              />
            </div>
          </div>
          <div style={{ display: 'flex', gap: 16 }}>
            <div style={{ flex: 1 }}>
              <Form.Select
                field='relay_format'
                label={t('中转格式')}
                style={{ width: '100%' }}
              >
                {RELAY_FORMAT_OPTIONS.map((opt) => (
                  <Select.Option key={opt.value} value={opt.value}>
                    {t(opt.label)}
                  </Select.Option>
                ))}
              </Form.Select>
            </div>
            <div style={{ flex: 1 }}>
              <Form.Select
                field='model_match_mode'
                label={t('匹配模式')}
                style={{ width: '100%' }}
              >
                {MODEL_MATCH_MODE_OPTIONS.map((opt) => (
                  <Select.Option key={opt.value} value={opt.value}>
                    {t(opt.label)}
                  </Select.Option>
                ))}
              </Form.Select>
            </div>
          </div>
          <Form.Input
            field='model_pattern'
            label={t('模型匹配规则')}
            placeholder={t('如 gpt-4*, claude-*，留空匹配全部')}
            extraText={t('用于匹配模型名称的表达式，留空表示匹配所有模型')}
          />
          <Form.TextArea
            field='param_override'
            label={t('参数覆写')}
            placeholder={t('JSON 格式，如 {"temperature": 0.7}')}
            autosize={{ minRows: 2, maxRows: 6 }}
            extraText={t('JSON 格式，覆写发送到上游的请求参数')}
          />
          <Form.TextArea
            field='header_override'
            label={t('请求头覆写')}
            placeholder={t('JSON 格式，如 {"X-Custom": "value"}')}
            autosize={{ minRows: 2, maxRows: 6 }}
            extraText={t('JSON 格式，覆写发送到上游的请求头')}
          />
          <div style={{ display: 'flex', gap: 16 }}>
            <div style={{ flex: 1 }}>
              <Form.Switch
                field='log_request'
                label={t('记录请求体')}
              />
            </div>
            <div style={{ flex: 1 }}>
              <Form.Switch
                field='log_response'
                label={t('记录响应体')}
              />
            </div>
          </div>
          <Form.InputNumber
            field='log_max_size'
            label={t('日志最大体积 (bytes)')}
            placeholder='4096'
            extraText={t('记录的请求/响应体最大字节数')}
          />
        </Form>
      </Modal>

      {/* 日志详情 Modal */}
      <Modal
        title={`${t('请求记录详情')} #${logDetail?.id || ''}`}
        visible={logDetailVisible}
        onCancel={() => setLogDetailVisible(false)}
        footer={null}
        style={{ maxWidth: 700 }}
        bodyStyle={{ maxHeight: '70vh', overflow: 'auto' }}
      >
        {logDetailLoading ? (
          <div style={{ textAlign: 'center', padding: 40 }}>
            {t('加载中...')}
          </div>
        ) : logDetail ? (
          <div>
            <Descriptions
              data={[
                { key: t('请求 ID'), value: logDetail.request_id },
                { key: t('规则 ID'), value: logDetail.request_rule_id },
                { key: t('用户 ID'), value: logDetail.user_id },
                { key: t('令牌 ID'), value: logDetail.token_id },
                { key: t('渠道 ID'), value: logDetail.channel_id },
                { key: t('模型'), value: logDetail.model_name },
                { key: t('状态码'), value: logDetail.status_code },
                { key: t('延迟'), value: formatLatency(logDetail.latency) },
              ]}
              style={{ marginBottom: 16 }}
            />
            {logDetail.request_body && (
              <div style={{ marginBottom: 16 }}>
                <Typography.Title heading={6}>
                  {t('请求体')}
                </Typography.Title>
                <pre
                  style={{
                    background: 'var(--semi-color-fill-0)',
                    padding: 12,
                    borderRadius: 6,
                    maxHeight: 240,
                    overflow: 'auto',
                    fontSize: 12,
                    whiteSpace: 'pre-wrap',
                    wordBreak: 'break-all',
                  }}
                >
                  {logDetail.request_body}
                </pre>
              </div>
            )}
            {logDetail.response_body && (
              <div>
                <Typography.Title heading={6}>
                  {t('响应体')}
                </Typography.Title>
                <pre
                  style={{
                    background: 'var(--semi-color-fill-0)',
                    padding: 12,
                    borderRadius: 6,
                    maxHeight: 240,
                    overflow: 'auto',
                    fontSize: 12,
                    whiteSpace: 'pre-wrap',
                    wordBreak: 'break-all',
                  }}
                >
                  {logDetail.response_body}
                </pre>
              </div>
            )}
          </div>
        ) : (
          <div style={{ textAlign: 'center', padding: 40, color: 'var(--semi-color-text-2)' }}>
            {t('暂无数据')}
          </div>
        )}
      </Modal>
    </div>
  );
};

export default RequestRule;
