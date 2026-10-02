<div align="center">

![NexusTok](/web/public/logo.png)

# NexusTok

🍥 **新一代大模型網關與AI資產管理系統**

<p align="center">
  繁體中文 |
  <a href="./README.zh_CN.md">简体中文</a> |
  <a href="./README.md">English</a> |
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
  <a href="https://hub.docker.com/r/c1cadabob/nexustok">
    <img src="https://img.shields.io/badge/docker-dockerHub-blue" alt="docker">
  </a>
</p>

<p align="center">
  <a href="#-快速開始">快速開始</a> •
  <a href="#-主要特性">主要特性</a> •
  <a href="#-部署">部署</a>
</p>

</div>

## 📝 項目說明

> [!IMPORTANT]
> - 本專案僅面向合法授權的 AI API 閘道、組織內部鑑權、多模型管理、用量統計、成本核算和私有化部署場景。
> - 使用者必須合法取得上游 API Key、帳號、模型服務或介面權限，並遵守上游服務條款及適用法律法規。
> - 使用者應確保其使用方式符合上游服務條款及適用法律法規。
> - 面向公眾提供生成式人工智慧服務時，使用者應遵守[《生成式人工智慧服務管理暫行辦法》](http://www.cac.gov.cn/2023-07/13/c_1690898327029107.htm)等監管要求，自行完成所在司法轄區要求的備案、許可、內容安全、實名、日誌留存、稅務和上游授權等合規義務。

## 🚀 快速開始

### 使用 Docker Compose（推薦）

```bash
# 複製項目並執行生產部署
git clone https://github.com/c1cadaBob/NexusTok.git
cd NexusTok
bash scripts/deploy.sh
```

<details>
<summary><strong>使用 Docker 命令</strong></summary>

```bash
# 單容器相容模式（SQLite，不含 PostgreSQL 和 Redis）
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

> 單容器命令不會啟動 PostgreSQL 和 Redis；完整生產部署請使用 Compose。
> SQLite 不會自動遷移到 PostgreSQL。

</details>

---

🎉 部署完成後，訪問 `http://localhost:3030` 即可使用！

> [!WARNING]
> 將本專案作為面向公眾的生成式 AI 服務或 API 轉售服務運營時，使用者應先完成備案、內容安全、實名、日誌留存、稅務、支付和上游授權等合規義務。

📖 更多部署方式請參考 [部署指南](https://docs.nexustok.ai/zh/docs/installation)

## ✨ 主要特性

> 詳細特性請參考 [特性說明](https://docs.nexustok.ai/zh/docs/guide/wiki/basic-concepts/features-introduction)

### 🎨 核心功能

| 特性 | 說明 |
|------|------|
| 🎨 全新 UI | 現代化的用戶界面設計 |
| 🌍 多語言 | 支援簡體中文、繁體中文、英文、法語、日語 |
| 🔄 數據兼容 | 完全兼容原版 One API 資料庫 |
| 📈 數據看板 | 視覺化控制檯與統計分析 |
| 🔒 權限管理 | 令牌分組、模型限制、用戶管理 |

### 💰 授權用量與成本管理

- ✅ 合法授權場景下的內部儲值與額度分配（易支付、Stripe）
- ✅ 組織內按次、按量或快取命中成本核算
- ✅ 支援 OpenAI、Azure、DeepSeek、Claude、Qwen 等模型的快取計費統計
- ✅ 面向內部管理或企業客戶的靈活計費策略配置

### 🔐 授權與安全

- 😈 Discord 授權登錄
- 🤖 LinuxDO 授權登錄
- 📱 Telegram 授權登錄
- 🔑 OIDC 統一認證
- 🔍 Key 查詢使用額度（配合 [NexusTok-key-tool](https://github.com/c1cadabob/nexustok-key-tool)）

### 🚀 高級功能

**API 格式支援：**
- ⚡ [OpenAI Responses](https://docs.nexustok.ai/zh/docs/api/ai-model/chat/openai/create-response)
- ⚡ [OpenAI Realtime API](https://docs.nexustok.ai/zh/docs/api/ai-model/realtime/create-realtime-session)（含 Azure）
- ⚡ [Claude Messages](https://docs.nexustok.ai/zh/docs/api/ai-model/chat/create-message)
- ⚡ [Google Gemini](https://docs.nexustok.ai/api/google-gemini-chat)
- 🔄 [Rerank 模型](https://docs.nexustok.ai/zh/docs/api/ai-model/rerank/create-rerank)（Cohere、Jina）

**智慧路由：**
- ⚖️ 管道加權隨機
- 🔄 失敗自動重試
- 🚦 用戶級別模型限流

**格式轉換：**
- 🔄 **OpenAI Compatible ⇄ Claude Messages**
- 🔄 **OpenAI Compatible → Google Gemini**
- 🔄 **Google Gemini → OpenAI Compatible** - 僅支援文本，暫不支援函數調用
- 🚧 **OpenAI Compatible ⇄ OpenAI Responses** - 開發中
- 🔄 **思考轉內容功能**

**Reasoning Effort 支援：**

<details>
<summary>查看詳細配置</summary>

**OpenAI 系列模型：**
- `o3-mini-high` - High reasoning effort
- `o3-mini-medium` - Medium reasoning effort
- `o3-mini-low` - Low reasoning effort
- `gpt-5-high` - High reasoning effort
- `gpt-5-medium` - Medium reasoning effort
- `gpt-5-low` - Low reasoning effort

**Claude 思考模型：**
- `claude-3-7-sonnet-20250219-thinking` - 啟用思考模式

**Google Gemini 系列模型：**
- `gemini-2.5-flash-thinking` - 啟用思考模式
- `gemini-2.5-flash-nothinking` - 禁用思考模式
- `gemini-2.5-pro-thinking` - 啟用思考模式
- `gemini-2.5-pro-thinking-128` - 啟用思考模式，並設置思考預算為128tokens
- 也可以直接在 Gemini 模型名稱後追加 `-low` / `-medium` / `-high` 來控制思考力道（無需再設置思考預算後綴）

</details>

## 🚢 部署

> [!TIP]
> **最新版 Docker 鏡像：** `c1cadabob/nexustok:latest`
>
> **v0.2.5 鏡像：** `c1cadabob/nexustok:v0.2.5`

### 📋 部署要求

| 組件 | 要求 |
|------|------|
| **生產預設資料庫** | PostgreSQL 15（應用同時支援 PostgreSQL ≥ 9.6） |
| **生產預設快取** | Redis 7（Compose 內部網路） |
| **相容資料庫** | MySQL ≥ 5.7.8、SQLite |
| **容器引擎** | Docker / Docker Compose |
| **系統架構** | 僅支援 64 位元系統（amd64 / arm64），不支援 32 位元系統 |

> Compose 是生產推薦入口。單容器命令僅代表 SQLite／無 Redis 相容模式，不會自動提供
> PostgreSQL 和 Redis；SQLite 也不會自動遷移到 PostgreSQL。

### ⚙️ 環境變數配置

<details>
<summary>常用環境變數配置</summary>

| 變數名 | 說明                                                           | 預設值 |
|--------|--------------------------------------------------------------|--------|
| `SESSION_SECRET` | 鑑權簽章密鑰；所有節點必須保持一致                                           | - |
| `SESSION_COOKIE_SECURE` | `false`/未設定時關閉 refresh/logout OriginGuard 以相容本機 HTTP 開發代理；`true` 時啟用 Secure Cookie 和嚴格 Origin 驗證 | `false` |
| `SESSION_COOKIE_TRUSTED_URL` | Secure 模式必填：允許呼叫 refresh/logout 的精確 HTTPS Origin，多個值以英文逗號分隔；不是 relay CORS 白名單 | - |
| `TRUSTED_PROXIES` | 未設定/留空時信任本機回送、RFC1918 和 IPv6 ULA 並輸出啟動警告；`none` 不信任任何代理；明確指定的代理 IP/CIDR 清單會完整取代預設值 | `127.0.0.0/8, ::1, 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, fc00::/7` |
| `USER_SESSION_ACTIVE_LIMIT` | 單一用戶最大活躍登入 Session 數 | `50` |
| `USER_SESSION_ISSUANCE_LIMIT` | 單一用戶在簽發視窗內可建立的 Session 總數，包含已撤銷 Session | `100` |
| `USER_SESSION_ISSUANCE_WINDOW_SECONDS` | Session 簽發計數視窗（秒）；高於 revoked 保留期時自動限制 | `86400` |
| `USER_SESSION_REVOKED_RETENTION_DAYS` | revoked Session 用於稽核與簽發計數的保留天數 | `7` |
| `USER_SESSION_HOURLY_ALERT_THRESHOLD` | 全域每小時 Session 簽發告警門檻；只告警，不拒絕登入 | `5000` |
| `CRYPTO_SECRET` | 快取鍵 HMAC 密鑰；共用 Redis 的節點必須使用相同有效值 | 預設跟隨 `SESSION_SECRET` |
| `SQL_DSN` | 資料庫連接字符串                                                     | - |
| `REDIS_CONN_STRING` | Redis 連接字符串                                                  | - |
| `STREAMING_TIMEOUT` | 流式超時時間（秒）                                                    | `300` |
| `STREAM_SCANNER_MAX_BUFFER_MB` | 流式掃描器單行最大緩衝（MB），圖像生成等超大 `data:` 片段（如 4K 圖片 base64）需適當調大 | `64` |
| `MAX_REQUEST_BODY_MB` | 請求體最大大小（MB，**解壓縮後**計；防止超大請求/zip bomb 導致記憶體暴漲），超過將返回 `413` | `32` |
| `AZURE_DEFAULT_API_VERSION` | Azure API 版本                                                 | `2025-04-01-preview` |
| `ERROR_LOG_ENABLED` | 錯誤日誌開關                                                       | `false` |
| `PYROSCOPE_URL` | Pyroscope 服務位址                                            | - |
| `PYROSCOPE_APP_NAME` | Pyroscope 應用名                                        | `NexusTok` |
| `PYROSCOPE_BASIC_AUTH_USER` | Pyroscope Basic Auth 用戶名                        | - |
| `PYROSCOPE_BASIC_AUTH_PASSWORD` | Pyroscope Basic Auth 密碼                  | - |
| `PYROSCOPE_MUTEX_RATE` | Pyroscope mutex 採樣率                               | `5` |
| `PYROSCOPE_BLOCK_RATE` | Pyroscope block 採樣率                               | `5` |
| `HOSTNAME` | Pyroscope 標籤裡的主機名                                          | `NexusTok` |

📖 **完整配置：** [環境變數文件](https://docs.nexustok.ai/zh/docs/installation/config-maintenance/environment-variables)

</details>

### 🖥️ 單機部署（PostgreSQL + Redis）

準備一台 64 位元 Linux 主機，以及 Docker Engine、Docker Compose v2、Git 和 `curl`。
防火牆只開放 `3030`；若使用 HTTPS 反向代理，則只開放代理使用的 `80/443`。
PostgreSQL `5432` 和 Redis `6379` 預設只在 Compose 內部網路存取，不應開放到公網。

```bash
git clone https://github.com/c1cadaBob/NexusTok.git
cd NexusTok
bash scripts/deploy.sh
```

首次執行會建立權限為 `0600` 的 `.env`，隨機產生 `POSTGRES_PASSWORD` 和
`REDIS_PASSWORD`；後續執行不會覆蓋既有密碼。腳本會先執行 `docker compose config`，
再拉取映像、啟動 PostgreSQL、Redis，並在啟動 NexusTok 前透過 Compose 網路執行真實
密碼驗證，隨後等待三個健康檢查通過。

```bash
docker compose ps
curl http://127.0.0.1:3030/api/status
curl http://127.0.0.1:3030/api/setup
```

確認狀態介面成功後，訪問 `http://伺服器位址:3030`，依設定精靈完成初始化並建立管理員
帳戶。反向代理必須轉發 SSE、WebSocket、長連線和 `X-Forwarded-*` 標頭。使用 HTTPS
時設定 `SESSION_COOKIE_SECURE=true`，並將 `SESSION_COOKIE_TRUSTED_URL` 設為實際 HTTPS
Origin；`TRUSTED_PROXIES` 只填寫可信反向代理的 IP/CIDR。

以下單容器命令只代表相容模式，不會啟動 PostgreSQL 或 Redis：

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
  c1cadabob/nexustok:v0.2.5
```

升級前備份 `.env`、`/opt/nexustok/data`、`/opt/nexustok/logs` 和 PostgreSQL named
volume，並優先匯出一致性資料庫備份：

```bash
docker compose exec -T postgres pg_dump -U root nexustok > nexustok-$(date +%F).sql
cp .env /secure-backup/nexustok.env
docker compose pull
docker compose up -d
```

回滾時將 NexusTok 映像固定為上一個已驗證的版本，執行 `docker compose up -d`，
並保留資料庫備份。不要刪除 PostgreSQL named volume。
SQLite 檔案不會自動遷移到 PostgreSQL；既有生產資料切換前必須備份並單獨完成驗證過的
資料遷移。

修改 `.env` 中的 `POSTGRES_PASSWORD` 不會修改既有 PostgreSQL named volume 中的資料庫
角色密碼。若腳本提示網路密碼驗證失敗，請先恢復原 `.env` 密碼，或在完成邏輯備份後
單獨同步資料庫角色密碼；不要只為了改密碼就刪除生產 volume。

常見問題包括 `3030` 被占用、PostgreSQL/Redis 不健康（查看
`docker compose logs postgres redis`）、`.env` 密碼被改動或為空、資料目錄權限或
SELinux/AppArmor 拒絕、amd64/arm64 映像不匹配，以及反向代理超時或未轉發 Upgrade 標頭。
全新部署清理前必須完成可用備份，只能精確刪除 NexusTok 的容器、volume、
`/opt/nexustok/data` 和 `/opt/nexustok/logs`。不要使用
`docker system prune -a`、`docker volume prune` 或未確認範圍的 `docker compose down -v`，
並保留 Komari 與其它非 NexusTok 資源。Docker socket 等同主機 Docker 管理權限，只應在
可信的管理伺服器掛載。

故障排除參考：[常見問題](https://docs.nexustok.ai/zh/docs/support/faq)。

### 🌐 多機部署

多機部署不要在每個應用節點啟動本文件內置的 PostgreSQL 和 Redis。應準備一套受內網
存取控制保護的共用 PostgreSQL 和共用 Redis，並為外部連線啟用 TLS。所有節點使用相同
的 `SQL_DSN`、`REDIS_CONN_STRING`、`SESSION_SECRET` 和 `CRYPTO_SECRET`，但
`NODE_NAME` 必須唯一。

主節點不設定 `NODE_TYPE=slave`，負責資料庫遷移和系統任務；從節點設定
`NODE_TYPE=slave`，只提供請求服務。所有節點保持時鐘同步，負載平衡器或反向代理使用
`/api/status` 做健康檢查，不健康節點應先摘除。

```bash
docker run --name nexustok-node-1 -d --restart always \
  -p 3030:3030 \
  --env-file /etc/nexustok/node.env \
  -e NODE_NAME=node-1 \
  -v /opt/nexustok/data:/data \
  -v /opt/nexustok/logs:/app/logs \
  -v /var/run/docker.sock:/var/run/docker.sock \
  c1cadabob/nexustok:v0.2.5
```

`node.env` 至少包含 `SQL_DSN`、`REDIS_CONN_STRING`、`SESSION_SECRET` 和
`CRYPTO_SECRET`；從節點再加入 `NODE_TYPE=slave`。主節點完成遷移並通過健康檢查後，
依序摘除從節點、更新、檢查並重新加入負載平衡器，最後在維護窗口處理主節點。回滾前
確認應用版本和資料庫遷移相容，並準備可用的 PostgreSQL 還原備份。

共用 Redis 可共用 Session、限流和快取控制面；每節點獨立 Redis 會造成狀態傳播延遲和
節點級限流；不使用 Redis 時 Session 回源資料庫、限流退回程序記憶體，叢集不具備全域
一致的限流計數。Docker socket、節點本地日誌和 `/data` 不會自動成為跨節點共用儲存，
集中日誌、共用檔案或任務產物必須另行設計。

### 🔄 管道重試與快取

**重試配置：** `設置 → 運營設置 → 通用設置 → 失敗重試次數`

**快取配置：**
- `REDIS_CONN_STRING`：Redis 快取（推薦）
- `MEMORY_CACHE_ENABLED`：記憶體快取

## 📜 許可證

本項目採用 [GNU Affero 通用公共許可證 v3.0 (AGPLv3)](./LICENSE) 授權。

本項目為開源項目，在 [One API](https://github.com/songquanpeng/one-api)（MIT 許可證）的基礎上進行二次開發。

如果您所在的組織政策不允許使用 AGPLv3 許可的軟體，或您希望規避 AGPLv3 的開源義務，請發送郵件至：[support@c1cadabob.dev](mailto:support@c1cadabob.dev)

<div align="center">

### 💖 感謝使用 NexusTok

如果這個項目對你有幫助，歡迎給我們一個 ⭐️ Star！

**[問題回饋](https://github.com/c1cadabob/nexustok/issues)** • **[最新發布](https://github.com/c1cadabob/nexustok/releases)**

<sub>Built with ❤️ by c1cadaBob</sub>

</div>
