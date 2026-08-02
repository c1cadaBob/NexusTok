import http from 'k6/http'
import { check, sleep } from 'k6'
import { Trend } from 'k6/metrics'

const BASE_URL = (__ENV.BASE_URL || 'http://127.0.0.1:3100').replace(/\/$/, '')
const API_TOKEN = __ENV.API_TOKEN || ''
const MODEL = __ENV.MODEL || 'gpt-loadtest'
const SCENARIO = __ENV.SCENARIO || 'mixed'
const VUS = Number.parseInt(__ENV.VUS || '20', 10)
const DURATION = __ENV.DURATION || '2m'
const RAMPING = (__ENV.RAMPING || 'false').toLowerCase() === 'true'
const RESULTS_DIR = __ENV.RESULTS_DIR || '/results/latest'

const statusDuration = new Trend('status_duration', true)
const chatNonStreamDuration = new Trend('chat_non_stream_duration', true)
const chatStreamFirstChunkMs = new Trend('chat_stream_first_chunk_ms', true)
const chatStreamDuration = new Trend('chat_stream_duration', true)

export const options = {
  scenarios: buildScenarios(),
  thresholds: buildThresholds(),
}

function buildScenarios() {
  const execName = ['status_smoke', 'chat_non_stream', 'chat_stream', 'mixed'].includes(SCENARIO)
    ? SCENARIO
    : 'mixed'

  if (RAMPING) {
    return {
      [execName]: {
        executor: 'ramping-vus',
        exec: execName,
        stages: [
          { duration: '30s', target: VUS },
          { duration: DURATION, target: VUS },
          { duration: '30s', target: 0 },
        ],
      },
    }
  }

  return {
    [execName]: {
      executor: 'constant-vus',
      exec: execName,
      vus: VUS,
      duration: DURATION,
    },
  }
}

function buildThresholds() {
  const thresholds = {
    http_req_failed: ['rate<0.01'],
  }
  if (SCENARIO === 'status_smoke') {
    thresholds.status_duration = ['p(95)<100']
    return thresholds
  }
  if (SCENARIO === 'chat_non_stream' || SCENARIO === 'mixed') {
    thresholds.chat_non_stream_duration = ['p(95)<1500']
  }
  if (SCENARIO === 'chat_stream' || SCENARIO === 'mixed') {
    thresholds.chat_stream_first_chunk_ms = ['p(95)<1000']
  }
  return thresholds
}

export function status_smoke() {
  const res = http.get(`${BASE_URL}/api/status`)
  statusDuration.add(res.timings.duration)
  check(res, {
    'status is 200': (r) => r.status === 200,
    'status response succeeds': (r) => String(r.body).includes('"success":true'),
  })
  sleep(1)
}

export function chat_non_stream() {
  const res = http.post(`${BASE_URL}/v1/chat/completions`, JSON.stringify(chatPayload(false)), requestParams())
  chatNonStreamDuration.add(res.timings.duration)

  let json = null
  try {
    json = res.json()
  } catch (_) {
    json = null
  }

  check(res, {
    'non-stream status is 200': (r) => r.status === 200,
    'non-stream has choices': () => Boolean(json && json.choices && json.choices.length > 0),
    'non-stream has usage': () => Boolean(json && json.usage && json.usage.total_tokens > 0),
  })
}

export function chat_stream() {
  const res = http.post(`${BASE_URL}/v1/chat/completions`, JSON.stringify(chatPayload(true)), requestParams())
  // k6 标准 HTTP API 会缓冲响应体；waiting 接近首字节时间，可作为 SSE TTFT 的轻量近似值。
  chatStreamFirstChunkMs.add(res.timings.waiting)
  chatStreamDuration.add(res.timings.duration)

  const body = String(res.body || '')
  check(res, {
    'stream status is 200': (r) => r.status === 200,
    'stream has SSE chunks': () => body.includes('data: {'),
    'stream has done marker': () => body.includes('data: [DONE]'),
  })
}

export function mixed() {
  if (Math.random() < 0.8) {
    chat_non_stream()
  } else {
    chat_stream()
  }
}

function chatPayload(stream) {
  return {
    model: MODEL,
    stream,
    messages: [
      { role: 'system', content: 'You are a deterministic load test assistant.' },
      { role: 'user', content: 'Return a short synthetic response for NexusTok load testing.' },
    ],
  }
}

function requestParams() {
  return {
    headers: {
      Authorization: `Bearer ${API_TOKEN}`,
      'Content-Type': 'application/json',
    },
    timeout: '600s',
  }
}

export function handleSummary(data) {
  return {
    stdout: renderConsoleSummary(data),
    [`${RESULTS_DIR}/summary.json`]: JSON.stringify(data, null, 2),
    [`${RESULTS_DIR}/summary.md`]: renderMarkdownSummary(data),
  }
}

function metricLine(data, name) {
  const metric = data.metrics[name]
  if (!metric || !metric.values) {
    return `- ${name}: n/a`
  }
  const p50 = metric.values.med ?? metric.values['p(50)']
  const p95 = metric.values['p(95)']
  const p99 = metric.values['p(99)']
  return `- ${name}: p50=${formatMetric(p50)} p95=${formatMetric(p95)} p99=${formatMetric(p99)}`
}

function formatMetric(value) {
  if (value === undefined || value === null || Number.isNaN(value)) {
    return 'n/a'
  }
  return `${Number(value).toFixed(2)}ms`
}

function renderConsoleSummary(data) {
  return [
    '',
    'NexusTok k6 summary',
    `scenario=${SCENARIO} vus=${VUS} duration=${DURATION} base_url=${BASE_URL}`,
    metricLine(data, 'http_req_duration'),
    metricLine(data, 'status_duration'),
    metricLine(data, 'chat_non_stream_duration'),
    metricLine(data, 'chat_stream_first_chunk_ms'),
    metricLine(data, 'chat_stream_duration'),
    '',
  ].join('\n')
}

function renderMarkdownSummary(data) {
  return [
    '# k6 Summary',
    '',
    `- Scenario: \`${SCENARIO}\``,
    `- VUs: \`${VUS}\``,
    `- Duration: \`${DURATION}\``,
    `- Base URL: \`${BASE_URL}\``,
    '',
    '## Latency',
    '',
    metricLine(data, 'http_req_duration'),
    metricLine(data, 'status_duration'),
    metricLine(data, 'chat_non_stream_duration'),
    metricLine(data, 'chat_stream_first_chunk_ms'),
    metricLine(data, 'chat_stream_duration'),
    '',
    '## Thresholds',
    '',
    'See `summary.json` for the full k6 threshold and check details.',
    '',
  ].join('\n')
}
