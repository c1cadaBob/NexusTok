<div align="center">

![nexustok](/web/default/public/logo.png)

# NexusTok

🍥 **Next-generation LLM gateway and AI asset management system**

<p align="center">
  <a href="./README.zh_CN.md">简体中文</a> |
  <strong>English</strong> |
  <a href="./README.zh_TW.md">繁體中文</a> |
  <a href="./README.fr.md">Français</a> |
  <a href="./README.ja.md">日本語</a>
</p>

<p align="center">
  <a href="#-quick-start">Quick Start</a> •
  <a href="#-key-capabilities">Key Capabilities</a> •
  <a href="#-deployment">Deployment</a> •
  <a href="#-documentation">Documentation</a> •
  <a href="#-license">License</a>
</p>

</div>

## 📝 Project Description

> [!IMPORTANT]
> - NexusTok is intended only for lawful and authorized AI API gateway, internal identity and access management, multi-provider routing, usage analytics, cost accounting, billing, and private deployment scenarios.
> - You must lawfully obtain upstream API keys, accounts, model services, and interface permissions, and you must comply with upstream terms of service and applicable laws and regulations.
> - If you provide generative AI services to the public, you are responsible for all compliance obligations required by your jurisdiction, including filing, licensing, content safety, real-name verification, log retention, tax, payment, and upstream authorization requirements.

NexusTok aggregates OpenAI-compatible, Claude, Gemini, Azure, AWS Bedrock, and other AI providers behind a unified API. It adds native account pools, quota and billing controls, model pricing, system task observability, traffic analytics, and an admin console for operators who need a self-hosted control plane.

---

## ✨ Key Capabilities

| Area | Capability |
|------|------------|
| Provider gateway | Unified relay for 40+ upstream AI providers, with OpenAI-compatible, Claude, Gemini, Azure, AWS Bedrock, Responses, and task-oriented routes. |
| Account pools | Native credential pools, account groups, OAuth/device authorization, health checks, usage logs, state audit logs, and channel-level pool binding. |
| Billing and quotas | Token/quota accounting, model pricing groups, dynamic billing expression support, subscription plans, wallet balance, redemption codes, and saturation-safe quota math. |
| Governance | JWT, OAuth, WebAuthn/Passkeys, admin permission catalog, route-level Authz enforcement, sensitive-field protection, and admin operation audit logs. |
| Operations | SystemTask runner, cross-node task locks, system instance heartbeat, log cleanup, channel testing, upstream model sync, account pool checks, and async task polling. |
| Analytics | Overview dashboard, model/user usage charts, Dashboard Flow Sankey analytics, usage logs, login audit logs, quota saturation visibility, and exportable records. |
| Payments | EPay, Stripe, Creem, Waffo, and Waffo Pancake configuration, catalog pairing, hosted checkout preparation, subscription product binding, and webhook-safe top-up paths. |
| Frontend | React 19 default console, classic console compatibility mode, mobile-friendly log/channel views, safe rich content rendering, Playground improvements, and i18n for en/zh/fr/ja/ru/vi. |
| Deployment | SQLite, MySQL >= 5.7.8, PostgreSQL >= 9.6, Redis or memory cache, Docker Compose, binary deployment, hot-reload development, and Electron desktop packaging notes. |

---

## 🚀 Quick Start

### Docker Compose (Recommended: PostgreSQL + Redis)

```bash
git clone https://github.com/c1cadaBob/NexusTok.git
cd NexusTok

# Compose starts PostgreSQL + Redis by default. Change the passwords and secrets in docker-compose.yml before production use.
nano docker-compose.yml

docker-compose up -d
```

After startup, open `http://localhost:3030` and complete the setup wizard.

<details>
<summary><strong>Docker command (production recommendation: external PostgreSQL + Redis)</strong></summary>

```bash
docker pull c1cadabob/nexustok:latest

docker run --name nexustok -d --restart always \
  -p 3030:3030 \
  -e TZ=Asia/Shanghai \
  -e PORT=3030 \
  -v ./data:/data \
  -v /var/run/docker.sock:/var/run/docker.sock \
  c1cadabob/nexustok:latest
```

`-v ./data:/data` stores SQLite and runtime data under the local `data` directory. This mode is a local/small-deployment fallback; use an absolute path if you choose it for a single-node deployment. `/var/run/docker.sock` enables dashboard Docker updates by allowing NexusTok to pull the image and recreate its container; it is equivalent to host Docker administration access and should only be mounted in trusted admin environments. Without it, NexusTok can still check updates but cannot apply Docker updates from the page.

The Docker image contains only the NexusTok application, not bundled database or Redis servers. For production single-container deployment, connect to external PostgreSQL and Redis through `SQL_DSN` and `REDIS_CONN_STRING`, for example:

```bash
-e SQL_DSN="postgresql://user:password@host:5432/nexustok?sslmode=disable"
-e REDIS_CONN_STRING="redis://:password@host:6379/0"
```

</details>

---

## 🚢 Deployment

| Component | Requirement |
|-----------|-------------|
| Production database | PostgreSQL >= 9.6 (recommended) |
| Compatible databases | SQLite, MySQL >= 5.7.8; SQLite is suitable for local trials and small single-node deployments |
| Production cache | Redis (recommended) |
| Small-deployment fallback | SQLite + memory cache, without external database or Redis |
| High-concurrency baseline | PostgreSQL primary database + Redis; consume logs can be split to an independent log database or ClickHouse |
| Runtime | Docker / Docker Compose or a Go binary deployment |

Common environment variables:

| Variable | Description |
|----------|-------------|
| `SESSION_SECRET` | Required for multi-instance deployments so sessions remain valid across nodes. |
| `CRYPTO_SECRET` | Required when Redis or encrypted shared data is enabled. |
| `SQL_DSN` | MySQL or PostgreSQL connection string. Leave empty for SQLite. |
| `LOG_SQL_DSN` | Independent log database connection string; supports MySQL/PostgreSQL, and consume logs can use ClickHouse. |
| `LOG_SQL_CLICKHOUSE_TTL_DAYS` | ClickHouse consume-log retention days; `0` disables TTL. |
| `REDIS_CONN_STRING` | Redis connection string. |
| `REDIS_POOL_SIZE` | Redis connection pool size; start with `128` or `256` for multi-node/high-concurrency tests. |
| `RELAY_MAX_IDLE_CONNS` | Maximum idle connections for Relay HTTP clients. |
| `RELAY_MAX_IDLE_CONNS_PER_HOST` | Maximum idle connections per upstream host. |
| `RELAY_MAX_CONNS_PER_HOST` | Maximum total connections per upstream host; `0` means unlimited. |
| `RELAY_RESPONSE_HEADER_TIMEOUT` | Timeout while waiting for upstream response headers, in seconds. Streaming after headers are received is not affected. |
| `RELAY_PROXY_CLIENT_CACHE_TTL` | Idle eviction TTL for HTTP clients cached by proxy URL, in seconds. |
| `RELAY_PROXY_CLIENT_CACHE_MAX_SIZE` | Maximum number of HTTP clients cached by proxy URL; `0` means unlimited. |
| `MAX_REQUEST_BODY_MB` | Decompressed request body size limit. |
| `STREAMING_TIMEOUT` | Streaming timeout in seconds. |
| `STREAM_SCANNER_MAX_BUFFER_MB` | Maximum scanner buffer for large streaming chunks. |

See [Deployment and Maintenance](./docs/installation/deployment.md) for production deployment, hot-reload development, binary deployment, theme switching, and troubleshooting.

---

## 📚 Documentation

| Topic | Link |
|-------|------|
| Deployment and maintenance | [docs/installation/deployment.md](./docs/installation/deployment.md) |
| Baota / BT panel installation | [docs/installation/BT.md](./docs/installation/BT.md) |
| Channel advanced settings | [docs/configuration/channel-other-settings.md](./docs/configuration/channel-other-settings.md) |
| Backend OpenAPI | [docs/openapi/api.json](./docs/openapi/api.json) |
| Relay OpenAPI | [docs/openapi/relay.json](./docs/openapi/relay.json) |
| Electron desktop package | [electron/README.md](./electron/README.md) |
| Issue tracker | [GitHub Issues](https://github.com/c1cadaBob/NexusTok/issues) |
| Releases | [GitHub Releases](https://github.com/c1cadaBob/NexusTok/releases) |

---

## 📜 License

NexusTok is licensed under [GNU Affero General Public License v3.0](./LICENSE).

This project is based on [One API](https://github.com/songquanpeng/one-api), which is licensed under the MIT License.

If your organization cannot use AGPLv3 software or you need a commercial license, contact [support@c1cada.dev](mailto:support@c1cada.dev).

---

<div align="center">

### Thanks for using NexusTok

**[Repository](https://github.com/c1cadaBob/NexusTok)** • **[Issues](https://github.com/c1cadaBob/NexusTok/issues)** • **[Releases](https://github.com/c1cadaBob/NexusTok/releases)**

<sub>Built by c1cada and contributors.</sub>

</div>
