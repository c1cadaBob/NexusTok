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
  IconCopy,
  IconFile,
  IconGithubLogo,
  IconPlay,
} from '@douyinfe/semi-icons';
import { Link } from 'react-router-dom';
import { DEFAULT_ENDPOINT } from '../../constants/common.constant';

const PROVIDER_NAMES = [
  'Moonshot',
  'OpenAI',
  'xAI',
  'Zhipu',
  'VolcEngine',
  'Cohere',
  'Claude',
  'Gemini',
  'Suno',
  'MiniMax',
  'Wenxin',
  'Spark',
  'Qingyan',
  'DeepSeek',
  'Qwen',
  'Midjourney',
  'Grok',
  'Azure AI',
  'Hunyuan',
  'Xinference',
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
  const providerNames = PROVIDER_NAMES.slice(0, isMobile ? 12 : 20);
  const requestPreview = buildRequestPreview(serverAddress, currentEndpoint);
  const heroStats = [
    {
      value: `${PROVIDER_NAMES.length}+`,
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
    <div
      className='w-full overflow-x-hidden bg-[#FFFFFF] text-[#111111]'
      style={{
        fontFamily: '"Helvetica Neue", Helvetica, Arial, sans-serif',
        letterSpacing: 0,
      }}
    >
      {/* Swiss 方向用网格线、编号和单一蓝色强调信息结构，避免再依赖渐变和软阴影。 */}
      <section className='relative overflow-hidden border-b border-[#111111] bg-[#F7F7F8] px-4 pb-12 pt-20 md:px-8 md:pb-16 md:pt-24'>
        <div className='pointer-events-none absolute inset-0 bg-[linear-gradient(to_right,#D9D9DE_1px,transparent_1px),linear-gradient(to_bottom,#D9D9DE_1px,transparent_1px)] bg-[size:64px_64px] opacity-70' />

        <div className='relative mx-auto grid w-full max-w-7xl min-w-0 gap-8 lg:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)] lg:gap-12'>
          <div className='grid min-w-0 gap-6 sm:grid-cols-[72px_minmax(0,1fr)]'>
            <div className='hidden border-l border-r border-[#111111] sm:block'>
              <div className='px-3 py-4 text-5xl font-black leading-none text-[#002FA7]'>
                01
              </div>
              <div className='border-t border-[#111111] px-3 py-4 text-xs font-bold'>
                NexusTok
              </div>
            </div>

            <div className='min-w-0'>
              <div className='mb-5 inline-flex border border-[#111111] bg-[#FFFFFF] px-3 py-1 text-xs font-bold text-[#002FA7]'>
                API Gateway
              </div>

              <h1 className='max-w-3xl text-5xl font-black leading-[0.98] text-[#111111] md:text-6xl lg:text-7xl'>
                {t('统一的')}
                <br />
                <span className='text-[#002FA7]'>{t('大模型接口网关')}</span>
              </h1>

              <p className='mt-6 max-w-xl text-base leading-7 text-[#333333] md:text-lg'>
                {t('多模型统一接入，只需将基址替换为：')}
              </p>

              <div className='mt-8 w-full max-w-2xl border border-[#111111] bg-[#FFFFFF]'>
                <div className='grid grid-cols-[minmax(0,1fr)_48px]'>
                  <input
                    id='classic-home-base-url'
                    name='base_url'
                    aria-label={t('API地址')}
                    readOnly
                    value={serverAddress}
                    className='h-12 min-w-0 border-0 bg-[#FFFFFF] px-4 font-mono text-sm text-[#111111] outline-none md:text-base'
                  />
                  <button
                    aria-label={t('复制')}
                    type='button'
                    onClick={handleCopyBaseURL}
                    className='flex h-12 w-12 items-center justify-center border-l border-[#111111] bg-[#002FA7] text-[#FFFFFF] transition-colors hover:bg-[#111111]'
                  >
                    <IconCopy />
                  </button>
                </div>
                <select
                  id='classic-home-endpoint'
                  name='endpoint'
                  aria-label={t('API端点')}
                  value={endpointIndex}
                  onChange={(event) =>
                    setEndpointIndex(Number(event.target.value))
                  }
                  className='h-11 w-full border-0 border-t border-[#111111] bg-[#F7F7F8] px-4 font-mono text-sm text-[#002FA7] outline-none'
                >
                  {endpointItems.map((item, index) => (
                    <option key={item.value} value={index}>
                      {item.value}
                    </option>
                  ))}
                </select>
              </div>

              <div className='mt-8 flex flex-wrap items-center gap-3'>
                <Link
                  to='/console'
                  className='inline-flex h-12 items-center gap-2 border border-[#002FA7] bg-[#002FA7] px-6 text-sm font-bold text-[#FFFFFF] transition-colors hover:bg-[#111111]'
                >
                  <IconPlay />
                  {t('获取密钥')}
                </Link>
                {isDemoSiteMode && version ? (
                  <button
                    type='button'
                    className='inline-flex h-12 items-center gap-2 border border-[#111111] bg-[#FFFFFF] px-5 text-sm font-bold text-[#111111] transition-colors hover:bg-[#111111] hover:text-[#FFFFFF]'
                    onClick={() =>
                      window.open(
                        'https://github.com/c1cada/NexusTok',
                        '_blank',
                      )
                    }
                  >
                    <IconGithubLogo />
                    {version}
                  </button>
                ) : (
                  docsLink && (
                    <button
                      type='button'
                      className='inline-flex h-12 items-center gap-2 border border-[#111111] bg-[#FFFFFF] px-5 text-sm font-bold text-[#111111] transition-colors hover:bg-[#111111] hover:text-[#FFFFFF]'
                      onClick={() => window.open(docsLink, '_blank')}
                    >
                      <IconFile />
                      {t('文档')}
                    </button>
                  )
                )}
              </div>

              <div className='mt-8 grid border-l border-t border-[#111111] bg-[#FFFFFF] sm:grid-cols-3'>
                {FEATURE_ITEMS.map((item, index) => (
                  <div
                    key={item.title}
                    className='min-h-[92px] border-b border-r border-[#111111] px-4 py-4'
                  >
                    <div className='text-2xl font-black text-[#002FA7]'>
                      {String(index + 1).padStart(2, '0')}
                    </div>
                    <div className='mt-3 text-sm font-bold text-[#111111]'>
                      {item.title}
                    </div>
                    <div className='mt-1 truncate font-mono text-xs text-[#555555]'>
                      {item.desc}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </div>

          <aside className='min-w-0 border border-[#111111] bg-[#FFFFFF]'>
            <div className='grid border-b border-[#111111] sm:grid-cols-[minmax(0,1fr)_180px]'>
              <div className='min-w-0 px-5 py-4'>
                <div className='text-4xl font-black leading-none text-[#002FA7]'>
                  02
                </div>
                <div className='mt-3 text-sm font-bold text-[#111111]'>
                  API Gateway
                </div>
                <div className='mt-1 truncate font-mono text-xs text-[#555555]'>
                  {currentEndpoint}
                </div>
              </div>
              <div className='flex items-center border-t border-[#111111] bg-[#002FA7] px-5 py-4 text-sm font-bold text-[#FFFFFF] sm:border-l sm:border-t-0'>
                {t('示例')} 200 OK
              </div>
            </div>

            <div className='grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] border-b border-[#111111] text-sm font-bold'>
              <div className='border-r border-[#111111] bg-[#002FA7] px-4 py-3 text-[#FFFFFF]'>
                {t('请求')}
              </div>
              <div className='border-r border-[#111111] px-4 py-3 text-[#111111]'>
                {t('响应')}
              </div>
              <div className='hidden px-4 py-3 text-[#555555] sm:block'>
                {t('流式')}
              </div>
            </div>

            <div className='grid min-h-[420px] grid-rows-[1fr_auto] font-mono text-xs leading-6'>
              <div className='px-5 py-5'>
                <div className='mb-4 flex items-center justify-between gap-4 border-b border-[#D9D9DE] pb-3 font-sans text-xs font-bold text-[#002FA7]'>
                  <span>{t('请求路径')}</span>
                  <span className='truncate font-mono text-[#111111]'>
                    {currentEndpoint}
                  </span>
                </div>
                <pre className='overflow-x-auto whitespace-pre-wrap text-[#111111]'>
                  {requestPreview}
                </pre>
              </div>

              <div className='border-t border-[#111111] bg-[#F7F7F8] px-5 py-5'>
                <div className='mb-4 border-b border-[#D9D9DE] pb-3 font-sans text-xs font-bold text-[#002FA7]'>
                  {t('示例')} {t('响应')}
                </div>
                <pre className='overflow-x-auto whitespace-pre-wrap text-[#111111]'>
                  {RESPONSE_PREVIEW}
                </pre>
              </div>
            </div>
          </aside>
        </div>

        <div className='relative mx-auto mt-10 grid max-w-7xl border-l border-t border-[#111111] bg-[#FFFFFF] md:grid-cols-3'>
          {heroStats.map((item, index) => (
            <div
              key={item.label}
              className='min-h-[132px] border-b border-r border-[#111111] px-5 py-5 text-left'
            >
              <div className='text-sm font-bold text-[#002FA7]'>
                {String(index + 3).padStart(2, '0')}
              </div>
              <div className='mt-4 text-4xl font-black leading-none text-[#111111] md:text-5xl'>
                {item.value}
              </div>
              <div className='mt-2 truncate text-sm text-[#555555]'>
                {item.label}
              </div>
            </div>
          ))}
        </div>

        <div className='relative mx-auto mt-10 max-w-7xl'>
          <div className='mb-4 flex items-end justify-between gap-4 border-b border-[#111111] pb-3'>
            <h2 className='text-xl font-black text-[#111111] md:text-2xl'>
              {t('支持众多的大模型供应商')}
            </h2>
            <span className='hidden text-4xl font-black leading-none text-[#002FA7] sm:block'>
              05
            </span>
          </div>
          <div className='grid border-l border-t border-[#111111] bg-[#FFFFFF] sm:grid-cols-4 lg:grid-cols-5'>
            {providerNames.map((name) => (
              <div
                key={name}
                className='flex min-h-[52px] items-center border-b border-r border-[#111111] px-4 text-sm font-bold text-[#111111]'
              >
                {name}
              </div>
            ))}
          </div>
        </div>
      </section>
    </div>
  );
}
