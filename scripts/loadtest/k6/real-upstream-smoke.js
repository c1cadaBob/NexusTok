import http from 'k6/http'
import { check, sleep } from 'k6'

const REAL_BASE_URL = (__ENV.REAL_BASE_URL || '').replace(/\/$/, '')
const REAL_API_KEY = __ENV.REAL_API_KEY || ''
const REAL_MODEL = __ENV.REAL_MODEL || ''
const ITERATIONS = Number.parseInt(__ENV.REAL_ITERATIONS || '10', 10)

if (!REAL_BASE_URL || !REAL_API_KEY || !REAL_MODEL) {
  throw new Error('REAL_BASE_URL、REAL_API_KEY、REAL_MODEL 必须全部显式提供，避免误用真实额度')
}

export const options = {
  scenarios: {
    real_upstream_smoke: {
      executor: 'shared-iterations',
      vus: 1,
      iterations: ITERATIONS,
      maxDuration: '5m',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.05'],
    http_req_duration: ['p(95)<10000'],
  },
}

export default function () {
  const res = http.post(
    `${REAL_BASE_URL}/v1/chat/completions`,
    JSON.stringify({
      model: REAL_MODEL,
      messages: [{ role: 'user', content: 'NexusTok real upstream smoke test. Reply with one short sentence.' }],
    }),
    {
      headers: {
        Authorization: `Bearer ${REAL_API_KEY}`,
        'Content-Type': 'application/json',
      },
      timeout: '60s',
    },
  )

  check(res, {
    'real upstream status is 200': (r) => r.status === 200,
    'real upstream has choices': (r) => String(r.body || '').includes('"choices"'),
  })
  sleep(1)
}
