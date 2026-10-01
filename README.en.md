<div align="center">

![NexusTok](/web/public/logo.png)

# NexusTok

🍥 **Next-Generation Large Model Gateway and AI Asset Management System**

<p align="center">
  <a href="./README.md">中文</a> | 
  <strong>English</strong> | 
  <a href="./README.fr.md">Français</a> | 
  <a href="./README.ja.md">日本語</a>
</p>

<p align="center">
  <a href="https://raw.githubusercontent.com/c1cadabob/nexustok/main/LICENSE">
    <img src="https://img.shields.io/github/license/c1cadabob/nexustok?color=brightgreen" alt="license">
  </a>
  <a href="https://github.com/c1cadabob/nexustok/releases/latest">
    <img src="https://img.shields.io/github/v/release/c1cadabob/nexustok?color=brightgreen&include_prereleases" alt="release">
  </a>
  <a href="https://github.com/users/c1cadabob/packages/container/package/NexusTok">
    <img src="https://img.shields.io/badge/docker-ghcr.io-blue" alt="docker">
  </a>
  <a href="https://hub.docker.com/r/c1cadabob/nexustok">
    <img src="https://img.shields.io/badge/docker-dockerHub-blue" alt="docker">
  </a>
</p>

<p align="center">
  <a href="#-quick-start">Quick Start</a> •
  <a href="#-key-features">Key Features</a> •
  <a href="#-deployment">Deployment</a>
</p>

</div>

## 📝 Project Description

> [!NOTE]  
> This is an open-source project developed based on [One API](https://github.com/songquanpeng/one-api)

> [!IMPORTANT]  
> - This project is intended solely for lawful and authorized AI API gateway, organization-level authentication, multi-model management, usage analytics, cost accounting, and private deployment scenarios.
> - Users must lawfully obtain upstream API keys, accounts, model services, and interface permissions, and must comply with upstream terms of service and applicable laws and regulations.
> - Users should ensure their use complies with upstream terms of service and applicable laws and regulations.
> - When providing generative AI services to the public, users should comply with applicable regulatory requirements and fulfill all filing, licensing, content safety, real-name verification, log retention, tax, and upstream authorization obligations required by their jurisdiction.

## 🚀 Quick Start

### Using Docker Compose (Recommended)

```bash
# Clone the project and run the production deployment
git clone https://github.com/c1cadaBob/NexusTok.git
cd NexusTok
bash scripts/deploy.sh
```

<details>
<summary><strong>Using Docker Commands</strong></summary>

```bash
# Pull the latest image
docker pull c1cadabob/nexustok:latest

# Single-container compatibility mode (SQLite, without PostgreSQL or Redis)
docker run --name NexusTok -d --restart always \
  -p 3030:3030 \
  -e TZ=Asia/Shanghai \
  -e PORT=3030 \
  -e SESSION_SECRET_FILE=/data/session_secret \
  -v ./data:/data \
  -v ./logs:/app/logs \
  -v /var/run/docker.sock:/var/run/docker.sock \
  c1cadabob/nexustok:latest
```

> **Tip:** The single-container command does not start PostgreSQL or Redis. Use Compose
> for the complete production topology. SQLite is not migrated to PostgreSQL automatically.

</details>

---

🎉 After deployment is complete, visit `http://localhost:3030` to start using!

> [!WARNING]
> When operating this project as a public generative AI service or API resale service, users should first complete all required filing, licensing, content safety, real-name verification, log retention, tax, payment, and upstream authorization obligations.

📖 For more deployment methods, please refer to [Deployment Guide](https://docs.nexustok.ai/en/docs/installation)

## ✨ Key Features

> For detailed features, please refer to [Features Introduction](https://docs.nexustok.ai/en/docs/guide/wiki/basic-concepts/features-introduction)

### 🎨 Core Functions

| Feature | Description |
|------|------|
| 🎨 New UI | Modern user interface design |
| 🌍 Multi-language | Supports Chinese, English, French, Japanese |
| 🔄 Data Compatibility | Fully compatible with the original One API database |
| 📈 Data Dashboard | Visual console and statistical analysis |
| 🔒 Permission Management | Token grouping, model restrictions, user management |

### 💰 Authorized Usage Accounting and Billing

- ✅ Internal top-up and quota allocation for lawful authorized scenarios (EPay, Stripe)
- ✅ Organization-level per-request, usage-based, and cache-hit cost accounting
- ✅ Cache billing statistics for OpenAI, Azure, DeepSeek, Claude, Qwen, and supported models
- ✅ Flexible billing policies for internal management or authorized enterprise customers

### 🔐 Authorization and Security

- 😈 Discord authorization login
- 🤖 LinuxDO authorization login
- 📱 Telegram authorization login
- 🔑 OIDC unified authentication

### 🚀 Advanced Features

**API Format Support:**
- ⚡ [OpenAI Responses](https://docs.nexustok.ai/en/docs/api/ai-model/chat/openai/create-response)
- ⚡ [OpenAI Realtime API](https://docs.nexustok.ai/en/docs/api/ai-model/realtime/create-realtime-session) (including Azure)
- ⚡ [Claude Messages](https://docs.nexustok.ai/en/docs/api/ai-model/chat/create-message)
- ⚡ [Google Gemini](https://docs.nexustok.ai/en/api/google-gemini-chat)
- 🔄 [Rerank Models](https://docs.nexustok.ai/en/docs/api/ai-model/rerank/create-rerank) (Cohere, Jina)

**Intelligent Routing:**
- ⚖️ Channel weighted random
- 🔄 Automatic retry on failure
- 🚦 User-level model rate limiting

**Format Conversion:**
- 🔄 **OpenAI Compatible ⇄ Claude Messages**
- 🔄 **OpenAI Compatible → Google Gemini**
- 🔄 **Google Gemini → OpenAI Compatible** - Text only, function calling not supported yet
- 🚧 **OpenAI Compatible ⇄ OpenAI Responses** - In development
- 🔄 **Thinking-to-content functionality**

**Reasoning Effort Support:**

<details>
<summary>View detailed configuration</summary>

**OpenAI series models:**
- `o3-mini-high` - High reasoning effort
- `o3-mini-medium` - Medium reasoning effort
- `o3-mini-low` - Low reasoning effort
- `gpt-5-high` - High reasoning effort
- `gpt-5-medium` - Medium reasoning effort
- `gpt-5-low` - Low reasoning effort

**Claude thinking models:**
- `claude-3-7-sonnet-20250219-thinking` - Enable thinking mode

**Google Gemini series models:**
- `gemini-2.5-flash-thinking` - Enable thinking mode
- `gemini-2.5-flash-nothinking` - Disable thinking mode
- `gemini-2.5-pro-thinking` - Enable thinking mode
- `gemini-2.5-pro-thinking-128` - Enable thinking mode with thinking budget of 128 tokens
- You can also append `-low`, `-medium`, or `-high` to any Gemini model name to request the corresponding reasoning effort (no extra thinking-budget suffix needed).

</details>

## 🚢 Deployment

> [!TIP]
> **Latest Docker image:** `c1cadabob/nexustok:latest`
>
> **v0.2.3 image:** `c1cadabob/nexustok:v0.2.3`

### 📋 Deployment Requirements

| Component | Requirement |
|------|------|
| **Production database** | PostgreSQL 15 (the application also supports PostgreSQL ≥ 9.6) |
| **Production cache** | Redis 7 on the internal Compose network |
| **Compatible databases** | MySQL ≥ 5.7.8 and SQLite |
| **Container engine** | Docker / Docker Compose |
| **System architecture** | 64-bit only (amd64 / arm64); 32-bit systems are not supported |

> Compose is the recommended production entry point. The single-container command is only
> a SQLite/no-Redis compatibility mode. PostgreSQL and Redis are not created automatically,
> and an existing SQLite file is never migrated to PostgreSQL automatically.

### ⚙️ Environment Variable Configuration

<details>
<summary>Common environment variable configuration</summary>

| Variable Name | Description | Default Value |
|--------|------|--------|
| `SESSION_SECRET` | Authentication signing secret; must be identical on every node | - |
| `SESSION_COOKIE_SECURE` | `false`/unset disables the refresh/logout OriginGuard for local HTTP dev proxies; `true` enables the Secure cookie and strict Origin checks | `false` |
| `SESSION_COOKIE_TRUSTED_URL` | Required with Secure mode: comma-separated exact HTTPS Origins allowed to call refresh/logout; not a relay CORS allowlist | - |
| `TRUSTED_PROXIES` | Unset/blank trusts loopback, RFC 1918 and IPv6 ULA with a startup warning; `none` trusts no proxies; an explicit proxy IP/CIDR list replaces the defaults | `127.0.0.0/8, ::1, 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, fc00::/7` |
| `USER_SESSION_ACTIVE_LIMIT` | Maximum active login Sessions per user | `50` |
| `USER_SESSION_ISSUANCE_LIMIT` | Maximum Sessions created per user within the issuance window, including revoked Sessions | `100` |
| `USER_SESSION_ISSUANCE_WINDOW_SECONDS` | Per-user Session issuance window; clamped to the revoked retention period when configured higher | `86400` |
| `USER_SESSION_REVOKED_RETENTION_DAYS` | Days to retain revoked Session rows for audit and issuance accounting | `7` |
| `USER_SESSION_HOURLY_ALERT_THRESHOLD` | Global Sessions created per hour that triggers an alert only; it never blocks login | `5000` |
| `CRYPTO_SECRET` | HMAC secret for cache keys; nodes sharing Redis must use the same effective value | Defaults to `SESSION_SECRET` |
| `SQL_DSN` | Database connection string | - |
| `REDIS_CONN_STRING` | Redis connection string | - |
| `STREAMING_TIMEOUT` | Streaming timeout (seconds) | `300` |
| `STREAM_SCANNER_MAX_BUFFER_MB` | Max per-line buffer (MB) for the stream scanner; increase when upstream sends huge image/base64 payloads | `64` |
| `MAX_REQUEST_BODY_MB` | Max request body size (MB, counted **after decompression**; prevents huge requests/zip bombs from exhausting memory). Exceeding it returns `413` | `32` |
| `AZURE_DEFAULT_API_VERSION` | Azure API version | `2025-04-01-preview` |
| `ERROR_LOG_ENABLED` | Error log switch | `false` |
| `PYROSCOPE_URL` | Pyroscope server address | - |
| `PYROSCOPE_APP_NAME` | Pyroscope application name | `NexusTok` |
| `PYROSCOPE_BASIC_AUTH_USER` | Pyroscope basic auth user | - |
| `PYROSCOPE_BASIC_AUTH_PASSWORD` | Pyroscope basic auth password | - |
| `PYROSCOPE_MUTEX_RATE` | Pyroscope mutex sampling rate | `5` |
| `PYROSCOPE_BLOCK_RATE` | Pyroscope block sampling rate | `5` |
| `HOSTNAME` | Hostname tag for Pyroscope | `NexusTok` |

📖 **Complete configuration:** [Environment Variables Documentation](https://docs.nexustok.ai/en/docs/installation/config-maintenance/environment-variables)

</details>

### 🖥️ Single-machine deployment (PostgreSQL + Redis)

Prepare a 64-bit Linux host with Docker Engine, Docker Compose v2, Git and `curl`. Open only
port `3030` in the firewall, or only `80/443` when an HTTPS reverse proxy is the public entry
point. PostgreSQL `5432` and Redis `6379` stay on the internal Compose network and must not be
exposed publicly.

```bash
git clone https://github.com/c1cadaBob/NexusTok.git
cd NexusTok
bash scripts/deploy.sh
```

On the first run the script creates a `.env` with mode `0600` and random
`POSTGRES_PASSWORD` and `REDIS_PASSWORD` values. Existing passwords are preserved. The script
validates `docker compose config`, pulls the image, starts PostgreSQL, Redis and NexusTok, and
waits for all three health checks.

```bash
docker compose ps
curl http://127.0.0.1:3030/api/status
curl http://127.0.0.1:3030/api/setup
```

After the status endpoint succeeds, open `http://server-address:3030`, complete the setup wizard
and create the administrator account. A reverse proxy must support SSE, WebSocket, long-lived
connections and the `X-Forwarded-*` headers. With HTTPS, set `SESSION_COOKIE_SECURE=true` and
set `SESSION_COOKIE_TRUSTED_URL` to the exact HTTPS Origin. Set `TRUSTED_PROXIES` only to the
trusted proxy IP/CIDR ranges.

The standalone command below is compatibility mode only:

```bash
mkdir -p /opt/nexustok/data /opt/nexustok/logs
docker run --name nexustok -d --restart always \
  -p 3030:3030 \
  -e TZ=Asia/Shanghai \
  -e PORT=3030 \
  -e SESSION_SECRET_FILE=/data/session_secret \
  -v /opt/nexustok/data:/data \
  -v /opt/nexustok/logs:/app/logs \
  -v /var/run/docker.sock:/var/run/docker.sock \
  c1cadabob/nexustok:v0.2.3
```

It does not start PostgreSQL or Redis. Before an upgrade, back up `.env`,
`/opt/nexustok/data`, `/opt/nexustok/logs` and the PostgreSQL named volume. Prefer a consistent
database dump:

```bash
docker compose exec -T postgres pg_dump -U root nexustok > nexustok-$(date +%F).sql
cp .env /secure-backup/nexustok.env
docker compose pull
docker compose up -d
```

For rollback, pin the NexusTok service to the previously verified release image, run
`docker compose up -d`, and keep the database backup. Do not
delete the PostgreSQL named volume. Existing SQLite data requires a separate, verified migration
before switching to PostgreSQL.

Common problems include port `3030` conflicts, unhealthy PostgreSQL/Redis containers
(`docker compose logs postgres redis`), changed or empty `.env` passwords, filesystem
permissions or SELinux/AppArmor denials, amd64/arm64 image mismatches, and reverse-proxy
timeouts or missing Upgrade headers for SSE/WebSocket. The Docker socket grants host-level Docker
management and should only be mounted on a trusted administrator-controlled server.

Troubleshooting reference: [FAQ](https://docs.nexustok.ai/en/docs/support/faq).

### 🌐 Multi-machine deployment

Do not start the bundled PostgreSQL and Redis on every application node. Prepare one shared
PostgreSQL service and one shared Redis service behind private-network access controls and TLS.
Every application node must use the same `SQL_DSN`, `REDIS_CONN_STRING`, `SESSION_SECRET` and
`CRYPTO_SECRET`, while `NODE_NAME` must be unique.

The primary node leaves `NODE_TYPE` unset and performs database migrations and system tasks.
Follower nodes set `NODE_TYPE=slave` and serve requests only. Synchronize clocks on every node.
Route traffic through a load balancer or reverse proxy and use `/api/status` as the health check;
remove an unhealthy node before maintenance.

Each node can run the application against the external services with an environment file:

```bash
docker run --name nexustok-node-1 -d --restart always \
  -p 3030:3030 \
  --env-file /etc/nexustok/node.env \
  -e NODE_NAME=node-1 \
  -v /opt/nexustok/data:/data \
  -v /opt/nexustok/logs:/app/logs \
  -v /var/run/docker.sock:/var/run/docker.sock \
  c1cadabob/nexustok:v0.2.3
```

`node.env` must contain at least `SQL_DSN`, `REDIS_CONN_STRING`, `SESSION_SECRET` and
`CRYPTO_SECRET`; add `NODE_TYPE=slave` on followers. After the primary has completed migrations
and passed its health check, upgrade followers one at a time, verify them, re-add them to the
load balancer, and handle the primary in the maintenance window. Confirm database compatibility
before rollback and keep a tested PostgreSQL restore.

Shared Redis provides shared Session, rate-limit and cache control-plane state. Independent Redis
instances create propagation delays and node-local limits. Without Redis, Sessions fall back to
the database and rate limits are process-local, so cluster-wide limits are not consistent. Docker
socket access, node-local logs and `/data` are not shared storage; centralized logs and shared
files require a separate design.

### 🔄 Channel Retry and Cache

**Retry configuration:** `Settings → Operation Settings → General Settings → Failure Retry Count`

**Cache configuration:**
- `REDIS_CONN_STRING`: Redis cache (recommended)
- `MEMORY_CACHE_ENABLED`: Memory cache

<div align="center">

### 💖 Thank you for using NexusTok

If this project is helpful to you, welcome to give us a ⭐️ Star！

**[Issue Feedback](https://github.com/c1cadabob/nexustok/issues)** • **[Latest Release](https://github.com/c1cadabob/nexustok/releases)**

<sub>Built with ❤️ by c1cadaBob</sub>

</div>
