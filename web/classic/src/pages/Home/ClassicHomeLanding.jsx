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

import {
  Button,
  Input,
  ScrollItem,
  ScrollList,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IconCopy,
  IconFile,
  IconGithubLogo,
  IconPlay,
} from '@douyinfe/semi-icons';
import { Link } from 'react-router-dom';
import {
  AzureAI,
  Claude,
  Cohere,
  DeepSeek,
  Gemini,
  Grok,
  Hunyuan,
  Midjourney,
  Minimax,
  Moonshot,
  OpenAI,
  Qingyan,
  Qwen,
  Spark,
  Suno,
  Volcengine,
  Wenxin,
  Xinference,
  XAI,
  Zhipu,
} from '@lobehub/icons';
import { DEFAULT_ENDPOINT } from '../../constants/common.constant';

const { Text } = Typography;

const PROVIDER_ICONS = [
  { name: 'Moonshot', icon: Moonshot },
  { name: 'OpenAI', icon: OpenAI },
  { name: 'xAI', icon: XAI },
  { name: 'Zhipu', icon: Zhipu.Color },
  { name: 'VolcEngine', icon: Volcengine.Color },
  { name: 'Cohere', icon: Cohere.Color },
  { name: 'Claude', icon: Claude.Color },
  { name: 'Gemini', icon: Gemini.Color },
  { name: 'Suno', icon: Suno },
  { name: 'MiniMax', icon: Minimax.Color },
  { name: 'Wenxin', icon: Wenxin.Color },
  { name: 'Spark', icon: Spark.Color },
  { name: 'Qingyan', icon: Qingyan.Color },
  { name: 'DeepSeek', icon: DeepSeek.Color },
  { name: 'Qwen', icon: Qwen.Color },
  { name: 'Midjourney', icon: Midjourney },
  { name: 'Grok', icon: Grok },
  { name: 'Azure AI', icon: AzureAI.Color },
  { name: 'Hunyuan', icon: Hunyuan.Color },
  { name: 'Xinference', icon: Xinference.Color },
];

const FEATURE_ITEMS = [
  {
    title: 'OpenAI-compatible',
    desc: '/v1/chat/completions',
  },
  {
    title: 'Claude-compatible',
    desc: '/v1/messages',
  },
  {
    title: 'Gemini-compatible',
    desc: '/v1beta/models',
  },
];

function buildRequestPreview(serverAddress, currentEndpoint) {
  return `curl -X POST "${serverAddress}${currentEndpoint}" \\
  -H "Authorization: Bearer sk-••••" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "your-model",
    "messages": [
      { "role": "user", "content": "..." }
    ]
  }'`;
}

const RESPONSE_PREVIEW = `{
  "choices": [{ "message": { "content": "routed" } }],
  "usage": { "total_tokens": 32 }
}`;

export function ClassicHomeLanding(props) {
  const {
    t,
    isChinese,
    isMobile,
    serverAddress,
    endpointItems,
    endpointIndex,
    setEndpointIndex,
    handleCopyBaseURL,
    isDemoSiteMode,
    docsLink,
    version,
  } = props;

  const currentEndpoint =
    endpointItems[endpointIndex]?.value || DEFAULT_ENDPOINT;
  const providerIcons = PROVIDER_ICONS.slice(0, isMobile ? 12 : 18);
  const requestPreview = buildRequestPreview(serverAddress, currentEndpoint);
  const heroStats = [
    {
      value: `${PROVIDER_ICONS.length}+`,
      label: t('支持众多的大模型供应商'),
    },
    {
      value: `${endpointItems.length}+`,
      label: t('API端点'),
    },
    {
      value: '200 OK',
      label: currentEndpoint,
    },
  ];

  return (
    <div className='w-full overflow-x-hidden'>
      {/* 首页首屏使用熟悉的 classic 组件，但改成更聚焦的产品化布局。 */}
      <div className='relative min-h-[calc(100vh-60px)] overflow-hidden border-b border-semi-color-border bg-semi-color-bg-0 px-4 pb-12 pt-20 md:px-8 md:pb-16 md:pt-24'>
        <div className='pointer-events-none absolute inset-0 -z-0 bg-[linear-gradient(to_right,var(--semi-color-border)_1px,transparent_1px),linear-gradient(to_bottom,var(--semi-color-border)_1px,transparent_1px)] bg-[size:5rem_5rem] opacity-[0.08]' />
        <div className='pointer-events-none absolute inset-x-0 top-0 -z-0 h-96 bg-[linear-gradient(to_bottom,var(--semi-color-fill-0),transparent)]' />

        <div className='relative z-10 mx-auto grid w-full max-w-7xl min-w-0 items-center gap-10 lg:grid-cols-[minmax(0,0.98fr)_minmax(0,1.02fr)] lg:gap-14'>
          <div className='w-full min-w-0 max-w-2xl'>
            <div className='mb-5 inline-flex items-center gap-2 rounded-lg border border-semi-color-border bg-semi-color-fill-0 px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em] text-semi-color-text-1'>
              <span className='h-2 w-2 rounded-full bg-semi-color-primary' />
              API Gateway
            </div>

            <h1
              className={`text-4xl font-bold leading-[1.05] text-semi-color-text-0 md:text-5xl lg:text-6xl ${isChinese ? 'tracking-wide md:tracking-wider' : ''}`}
            >
              {t('统一的')}
              <br />
              <span className='shine-text'>{t('大模型接口网关')}</span>
            </h1>

            <p className='mt-6 max-w-xl text-base leading-7 text-semi-color-text-1 md:text-lg'>
              {t('多模型统一接入，只需将基址替换为：')}
            </p>

            <div className='mt-6 w-full max-w-xl rounded-lg border border-semi-color-border bg-semi-color-bg-0 p-1 shadow-[0_18px_50px_-36px_rgba(15,23,42,0.5)]'>
              <Input
                readonly
                value={serverAddress}
                className='w-full !rounded-lg'
                size={isMobile ? 'default' : 'large'}
                suffix={
                  <div className='flex min-w-0 items-center gap-2'>
                    <div className='hidden min-w-0 max-w-[180px] md:block'>
                      <ScrollList
                        bodyHeight={32}
                        style={{ border: 'unset', boxShadow: 'unset' }}
                      >
                        <ScrollItem
                          mode='wheel'
                          cycled={true}
                          list={endpointItems}
                          selectedIndex={endpointIndex}
                          onSelect={({ index }) => setEndpointIndex(index)}
                        />
                      </ScrollList>
                    </div>
                    <Button
                      type='primary'
                      onClick={handleCopyBaseURL}
                      icon={<IconCopy />}
                      className='!rounded-lg'
                    />
                  </div>
                }
              />
            </div>

            <div className='mt-8 flex flex-wrap items-center gap-3'>
              <Link to='/console'>
                <Button
                  theme='solid'
                  type='primary'
                  size={isMobile ? 'default' : 'large'}
                  className='!rounded-lg px-7 py-2'
                  icon={<IconPlay />}
                >
                  {t('获取密钥')}
                </Button>
              </Link>
              {isDemoSiteMode && version ? (
                <Button
                  size={isMobile ? 'default' : 'large'}
                  className='flex items-center !rounded-lg px-6 py-2'
                  icon={<IconGithubLogo />}
                  onClick={() =>
                    window.open('https://github.com/c1cada/NexusTok', '_blank')
                  }
                >
                  {version}
                </Button>
              ) : (
                docsLink && (
                  <Button
                    size={isMobile ? 'default' : 'large'}
                    className='flex items-center !rounded-lg px-6 py-2'
                    icon={<IconFile />}
                    onClick={() => window.open(docsLink, '_blank')}
                  >
                    {t('文档')}
                  </Button>
                )
              )}
            </div>

            <div className='mt-8 grid gap-3 sm:grid-cols-3'>
              {FEATURE_ITEMS.map((item) => (
                <div
                  key={item.title}
                  className='rounded-lg border border-semi-color-border bg-semi-color-bg-0 px-4 py-3 shadow-[0_10px_32px_-24px_rgba(15,23,42,0.45)]'
                >
                  <div className='text-sm font-semibold text-semi-color-text-0'>
                    {item.title}
                  </div>
                  <div className='mt-1 truncate font-mono text-xs text-semi-color-text-2'>
                    {item.desc}
                  </div>
                </div>
              ))}
            </div>
          </div>

          <div className='min-w-0'>
            <div className='rounded-lg border border-semi-color-border bg-semi-color-bg-0 p-3 shadow-[0_28px_80px_-42px_rgba(15,23,42,0.55)]'>
              <div className='rounded-lg border border-semi-color-border bg-semi-color-fill-0 px-4 py-3'>
                <div className='flex items-center justify-between gap-4'>
                  <div className='min-w-0'>
                    <div className='text-xs font-bold uppercase tracking-[0.18em] text-semi-color-text-0'>
                      API Gateway
                    </div>
                    <div className='mt-1 truncate font-mono text-xs text-semi-color-text-2'>
                      {currentEndpoint}
                    </div>
                  </div>
                  <div className='shrink-0 rounded-lg bg-semi-color-bg-0 px-3 py-1 font-mono text-xs font-semibold text-semi-color-primary'>
                    200 OK
                  </div>
                </div>
              </div>

              <div className='mt-3 overflow-hidden rounded-lg border border-semi-color-border bg-semi-color-bg-0'>
                <div className='flex items-center gap-2 border-b border-semi-color-border px-4 py-3 text-xs'>
                  <span className='rounded-lg bg-semi-color-primary px-3 py-1 font-semibold text-semi-color-bg-0'>
                    {t('请求')}
                  </span>
                  <span className='rounded-lg px-3 py-1 text-semi-color-text-2'>
                    {t('响应')}
                  </span>
                  <span className='ml-auto hidden text-semi-color-text-2 sm:inline'>
                    128 ms · {t('流式')}
                  </span>
                </div>

                <div className='grid min-h-[360px] grid-rows-[1fr_auto] font-mono text-xs leading-6'>
                  <div className='px-5 py-5'>
                    <div className='mb-3 text-[10px] font-semibold uppercase tracking-[0.18em] text-semi-color-text-2'>
                      {t('请求')}
                    </div>
                    <pre className='overflow-x-auto whitespace-pre-wrap text-semi-color-text-1'>
                      {requestPreview}
                    </pre>
                  </div>

                  <div className='border-t border-semi-color-border bg-semi-color-fill-0 px-5 py-4'>
                    <div className='mb-3 text-[10px] font-semibold uppercase tracking-[0.18em] text-semi-color-text-2'>
                      {t('响应')}
                    </div>
                    <pre className='overflow-x-auto whitespace-pre-wrap text-semi-color-text-1'>
                      {RESPONSE_PREVIEW}
                    </pre>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div className='relative z-10 mx-auto mt-10 max-w-7xl'>
          <div className='grid overflow-hidden rounded-lg border border-semi-color-border bg-semi-color-bg-0 shadow-[0_18px_60px_-38px_rgba(15,23,42,0.5)] md:grid-cols-3'>
            {heroStats.map((item) => (
              <div
                key={item.label}
                className='flex min-h-[96px] flex-col items-center justify-center border-b border-semi-color-border px-4 py-5 text-center last:border-b-0 md:border-b-0 md:border-r md:last:border-r-0'
              >
                <div className='text-2xl font-bold text-semi-color-text-0 md:text-3xl'>
                  {item.value}
                </div>
                <div className='mt-1 max-w-[220px] truncate text-xs text-semi-color-text-2'>
                  {item.label}
                </div>
              </div>
            ))}
          </div>
        </div>

        <div className='relative z-10 mx-auto mt-10 max-w-6xl text-center'>
          <Text type='tertiary' className='text-base md:text-lg'>
            {t('支持众多的大模型供应商')}
          </Text>
          <div className='mt-6 flex flex-wrap items-center justify-center gap-3 sm:gap-4 md:gap-5'>
            {providerIcons.map((provider) => {
              const ProviderIcon = provider.icon;
              return (
                <div
                  key={provider.name}
                  className='flex h-12 w-12 items-center justify-center rounded-lg border border-semi-color-border bg-semi-color-bg-0 shadow-[0_12px_30px_-24px_rgba(15,23,42,0.45)]'
                  title={provider.name}
                >
                  <ProviderIcon size={isMobile ? 28 : 34} />
                </div>
              );
            })}
            <div className='flex h-12 w-12 items-center justify-center rounded-lg border border-semi-color-border bg-semi-color-fill-0'>
              <Typography.Text className='!text-lg font-bold'>
                30+
              </Typography.Text>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
