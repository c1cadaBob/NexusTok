<div align="center">

![nexustok](/web/default/public/logo.png)

# NexusTok

🍥 **新一代大模型閘道與 AI 資產管理系統**

<p align="center">
  <a href="./README.zh_CN.md">简体中文</a> |
  <a href="./README.en.md">English</a> |
  <strong>繁體中文</strong> |
  <a href="./README.fr.md">Français</a> |
  <a href="./README.ja.md">日本語</a>
</p>

<p align="center">
  <a href="#-快速開始">快速開始</a> •
  <a href="#-核心能力">核心能力</a> •
  <a href="#-部署">部署</a> •
  <a href="#-文件">文件</a> •
  <a href="#-授權">授權</a>
</p>

</div>

## 📝 專案說明

> [!IMPORTANT]
> - NexusTok 僅面向合法授權的 AI API 閘道、組織內部鑑權、多提供商路由、用量分析、成本核算、計費和私有化部署場景。
> - 使用者必須合法取得上游 API Key、帳號、模型服務或介面權限，並遵守上游服務條款及適用法律法規。
> - 面向公眾提供生成式人工智慧服務時，使用者應自行完成所在司法轄區要求的備案、許可、內容安全、實名、日誌留存、稅務、支付和上游授權等合規義務。

NexusTok 將 OpenAI 相容介面、Claude、Gemini、Azure、AWS Bedrock 等上游 AI 服務聚合到統一 API 之下，並提供帳號池、額度與計費、模型定價、系統任務觀測、流量分析和管理後台，適合作為自託管的 AI 控制平面。

---

## ✨ 核心能力

| 領域 | 能力 |
|------|------|
| 提供商閘道 | 聚合 40+ 上游 AI 提供商，支援 OpenAI 相容、Claude、Gemini、Azure、AWS Bedrock、Responses 和任務類介面。 |
| 帳號池 | 原生憑證池、帳號分組、OAuth/裝置授權、健康檢查、使用日誌、狀態稽核和渠道級帳號池綁定。 |
| 計費與額度 | Token/額度核算、模型定價組、動態計費表達式、訂閱方案、錢包餘額、兌換碼和飽和保護額度計算。 |
| 治理與權限 | JWT、OAuth、WebAuthn/Passkeys、管理員權限 catalog、路由級 Authz enforcement、敏感欄位保護和管理稽核日誌。 |
| 運維能力 | SystemTask runner、跨節點任務鎖、系統實例心跳、日誌清理、渠道測試、內置模型倉庫、上游模型同步、帳號池檢測和非同步任務輪詢。 |
| 數據分析 | 總覽看板、模型/使用者用量圖、Dashboard Flow 桑基圖、使用日誌、登入稽核、額度飽和可觀測性和記錄匯出。 |
| 支付能力 | EPay、Stripe、Creem、Waffo、Waffo Pancake 配置，catalog/pair 綁定、託管支付準備、訂閱商品綁定和 webhook 冪等入帳。 |
| 前端體驗 | React 19 預設控制台、classic 相容主題、行動端日誌/渠道視圖、安全富文字渲染、Playground 增強和 en/zh/fr/ja/ru/vi 國際化。 |
| 部署方式 | SQLite、MySQL >= 5.7.8、PostgreSQL >= 9.6，Redis 或記憶體快取，Docker Compose、二進位部署、熱更新開發環境和 Electron 桌面端打包說明。 |

---

## 🚀 快速開始

### Docker Compose（推薦：PostgreSQL + Redis）

```bash
git clone https://github.com/c1cadaBob/NexusTok.git
cd NexusTok

# Compose 預設啟動 PostgreSQL + Redis；生產使用前請修改 docker-compose.yml 中的密碼和密鑰。
nano docker-compose.yml

docker-compose up -d
```

啟動後訪問 `http://localhost:3030`，按初始化嚮導完成配置。

<details>
<summary><strong>Docker 命令（生產推薦外接 PostgreSQL + Redis）</strong></summary>

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

`-v ./data:/data` 會把 SQLite 和執行時資料保存到目前目錄的 `data` 資料夾。該模式適合作為本地體驗或小規模單機 fallback；如選擇單機部署，生產環境建議使用絕對路徑。`/var/run/docker.sock` 用於系統維護頁內自動拉取映像並重建容器，等同宿主機 Docker 管理權限，只應在可信管理員環境中掛載；不掛載時仍可檢查更新，但不能在頁面內套用 Docker 更新。

Docker 映像只包含 NexusTok 應用程式，不內建資料庫或 Redis 服務。生產單容器推薦外接 PostgreSQL + Redis，並透過 `SQL_DSN` 和 `REDIS_CONN_STRING` 指定連線字串，例如：

```bash
-e SQL_DSN="postgresql://user:password@host:5432/nexustok?sslmode=disable"
-e REDIS_CONN_STRING="redis://:password@host:6379/0"
```

</details>

---

## 🚢 部署

| 元件 | 要求 |
|------|------|
| 生產主資料庫 | PostgreSQL >= 9.6（推薦） |
| 相容資料庫 | SQLite、MySQL >= 5.7.8；SQLite 適合本地體驗和小規模單機部署 |
| 生產快取 | Redis（推薦） |
| 小規模 fallback | SQLite + 記憶體快取，不需要外部資料庫和 Redis |
| 執行方式 | Docker / Docker Compose 或 Go 二進位部署 |

常用環境變數：

| 變數 | 說明 |
|------|------|
| `SESSION_SECRET` | 多實例部署必須設定，保證會話在節點間一致。 |
| `CRYPTO_SECRET` | 啟用 Redis 或共享加密資料時必須設定。 |
| `SQL_DSN` | MySQL 或 PostgreSQL 連線字串；為空時使用 SQLite。 |
| `REDIS_CONN_STRING` | Redis 連線字串。 |
| `MAX_REQUEST_BODY_MB` | 解壓後的請求體大小限制。 |
| `STREAMING_TIMEOUT` | 串流回應逾時時間（秒）。 |
| `STREAM_SCANNER_MAX_BUFFER_MB` | 大型串流片段的掃描緩衝上限。 |

生產部署、熱重載開發、二進位部署、主題切換和排障請參考 [啟動與部署維護手冊](./docs/installation/deployment.md)。

---

## 📚 文件

| 主題 | 連結 |
|------|------|
| 啟動與部署維護 | [docs/installation/deployment.md](./docs/installation/deployment.md) |
| 寶塔面板安裝 | [docs/installation/BT.md](./docs/installation/BT.md) |
| 渠道進階設定 | [docs/configuration/channel-other-settings.md](./docs/configuration/channel-other-settings.md) |
| 後端 OpenAPI | [docs/openapi/api.json](./docs/openapi/api.json) |
| Relay OpenAPI | [docs/openapi/relay.json](./docs/openapi/relay.json) |
| Electron 桌面端 | [electron/README.md](./electron/README.md) |
| 問題回報 | [GitHub Issues](https://github.com/c1cadaBob/NexusTok/issues) |
| 最新發布 | [GitHub Releases](https://github.com/c1cadaBob/NexusTok/releases) |

---

## 📜 授權

NexusTok 採用 [GNU Affero 通用公共授權條款 v3.0](./LICENSE) 授權。

本專案基於 [One API](https://github.com/songquanpeng/one-api)（MIT 授權）進行二次開發。

如果您所在的組織政策不允許使用 AGPLv3 授權的軟體，或您需要商業授權，請聯絡 [support@c1cada.dev](mailto:support@c1cada.dev)。

---

<div align="center">

### 感謝使用 NexusTok

**[專案倉庫](https://github.com/c1cadaBob/NexusTok)** • **[問題回報](https://github.com/c1cadaBob/NexusTok/issues)** • **[最新發布](https://github.com/c1cadaBob/NexusTok/releases)**

<sub>由 c1cada 與貢獻者共同構建。</sub>

</div>
