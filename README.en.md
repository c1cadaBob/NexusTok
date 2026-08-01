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

### Docker Compose (Recommended)

```bash
git clone https://github.com/c1cada/NexusTok.git
cd NexusTok

# Review docker-compose.yml before production use.
nano docker-compose.yml

docker-compose up -d
```

After startup, open `http://localhost:3000` and complete the setup wizard.

<details>
<summary><strong>Docker command</strong></summary>

```bash
docker pull c1cadabob/nexustok:latest

docker run --name nexustok -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  c1cadabob/nexustok:latest
```

`-v ./data:/data` stores SQLite and runtime data under the local `data` directory. Use an absolute path for production deployments.

</details>

---

## 🚢 Deployment

| Component | Requirement |
|-----------|-------------|
| Local database | SQLite with a persisted `/data` volume |
| Remote database | MySQL >= 5.7.8 or PostgreSQL >= 9.6 |
| Cache | Redis recommended; memory cache is available for small deployments |
| Runtime | Docker / Docker Compose or a Go binary deployment |

Common environment variables:

| Variable | Description |
|----------|-------------|
| `SESSION_SECRET` | Required for multi-instance deployments so sessions remain valid across nodes. |
| `CRYPTO_SECRET` | Required when Redis or encrypted shared data is enabled. |
| `SQL_DSN` | MySQL or PostgreSQL connection string. Leave empty for SQLite. |
| `REDIS_CONN_STRING` | Redis connection string. |
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
| Issue tracker | [GitHub Issues](https://github.com/c1cada/NexusTok/issues) |
| Releases | [GitHub Releases](https://github.com/c1cada/NexusTok/releases) |

---

## 📜 License

NexusTok is licensed under [GNU Affero General Public License v3.0](./LICENSE).

This project is based on [One API](https://github.com/songquanpeng/one-api), which is licensed under the MIT License.

If your organization cannot use AGPLv3 software or you need a commercial license, contact [support@c1cada.dev](mailto:support@c1cada.dev).

---

<div align="center">

### Thanks for using NexusTok

**[Repository](https://github.com/c1cada/NexusTok)** • **[Issues](https://github.com/c1cada/NexusTok/issues)** • **[Releases](https://github.com/c1cada/NexusTok/releases)**

<sub>Built by c1cada and contributors.</sub>

</div>
