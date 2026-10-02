<div align="center">

![NexusTok](/web/public/logo.png)

# NexusTok

🍥 **次世代大規模モデルゲートウェイとAI資産管理システム**

<p align="center">
  <a href="./README.zh_CN.md">简体中文</a> |
  <a href="./README.zh_TW.md">繁體中文</a> |
  <a href="./README.md">English</a> |
  <a href="./README.fr.md">Français</a> |
  <strong>日本語</strong>
</p>

<p align="center">
  <a href="https://raw.githubusercontent.com/c1cadabob/nexustok/main/LICENSE">
    <img src="https://img.shields.io/github/license/c1cadabob/nexustok?color=brightgreen" alt="license">
  </a><!--
  --><a href="https://github.com/c1cadabob/nexustok/releases/latest">
    <img src="https://img.shields.io/github/v/release/c1cadabob/nexustok?color=brightgreen&include_prereleases" alt="release">
  </a><!--
  --><a href="https://hub.docker.com/r/c1cadabob/nexustok">
    <img src="https://img.shields.io/badge/docker-dockerHub-blue" alt="docker">
  </a>
</p>

<p align="center">
  <a href="#-クイックスタート">クイックスタート</a> •
  <a href="#-主な機能">主な機能</a> •
  <a href="#-デプロイ">デプロイ</a>
</p>

</div>

## 📝 プロジェクト説明

> [!IMPORTANT]
> - 本プロジェクトは、合法的に許可された AI API ゲートウェイ、組織レベルの認証、マルチモデル管理、利用量分析、コスト管理、プライベートデプロイのシナリオのみを対象としています。
> - ユーザーは、上流の API キー、アカウント、モデルサービス、インターフェース権限を合法的に取得し、上流のサービス利用規約および適用される法律法規を遵守する必要があります。
> - ユーザーは、利用方法が上流のサービス利用規約および適用される法律法規に準拠していることを確認してください。
> - 生成 AI サービスを公衆に提供する場合、ユーザーは適用される規制要件を遵守し、管轄区域で求められる届出、ライセンス、コンテンツセキュリティ、本人確認、ログ保持、税務、上流認可などのすべての義務を履行してください。

## 🚀 クイックスタート

### Docker Composeを使用（推奨）

```bash
# プロジェクトをクローンして本番デプロイを実行
git clone https://github.com/c1cadaBob/NexusTok.git
cd NexusTok
bash scripts/deploy.sh
```

<details>
<summary><strong>Dockerコマンドを使用</strong></summary>

```bash
# 単一コンテナ互換モード（PostgreSQLとRedisなしのSQLite）
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

> このコマンドは PostgreSQL と Redis を起動しません。完全な本番構成には Compose を
> 使用してください。SQLite は PostgreSQL に自動移行されません。

</details>

---

🎉 デプロイが完了したら、`http://localhost:3030` にアクセスして使用を開始してください！

> [!WARNING]
> 本プロジェクトを公衆向け生成 AI サービスまたは API 再販サービスとして運営する場合、ユーザーは届出、コンテンツセキュリティ、本人確認、ログ保持、税務、決済、上流認可などの必要なコンプライアンス義務を先に完了してください。

📖 その他のデプロイ方法については[デプロイガイド](https://docs.nexustok.ai/ja/docs/installation)を参照してください。

## ✨ 主な機能

> 詳細な機能については[機能説明](https://docs.nexustok.ai/ja/docs/guide/wiki/basic-concepts/features-introduction)を参照してください。

### 🎨 コア機能

| 機能 | 説明 |
|------|------|
| 🎨 新しいUI | モダンなユーザーインターフェースデザイン |
| 🌍 多言語 | 簡体字中国語、繁体字中国語、英語、フランス語、日本語をサポート |
| 🔄 データ互換性 | オリジナルのOne APIデータベースと完全に互換性あり |
| 📈 データダッシュボード | ビジュアルコンソールと統計分析 |
| 🔒 権限管理 | トークングループ化、モデル制限、ユーザー管理 |

### 💰 認可済み利用量とコスト管理

- ✅ 合法的に許可されたシナリオでの内部チャージとクォータ割り当て（EPay、Stripe）
- ✅ 組織レベルのリクエスト単位、使用量ベース、キャッシュヒットのコスト会計
- ✅ OpenAI、Azure、DeepSeek、Claude、Qwen などのモデルのキャッシュ課金統計
- ✅ 内部管理または認可済み企業顧客向けの柔軟な課金ポリシー

### 🔐 認証とセキュリティ

- 😈 Discord認証ログイン
- 🤖 LinuxDO認証ログイン
- 📱 Telegram認証ログイン
- 🔑 OIDC統一認証
- 🔍 Key使用量クォータ照会（[NexusTok-key-tool](https://github.com/c1cadabob/nexustok-key-tool)と併用）



### 🚀 高度な機能

**APIフォーマットサポート:**
- ⚡ [OpenAI Responses](https://docs.nexustok.ai/ja/docs/api/ai-model/chat/openai/create-response)
- ⚡ [OpenAI Realtime API](https://docs.nexustok.ai/ja/docs/api/ai-model/realtime/create-realtime-session)（Azureを含む）
- ⚡ [Claude Messages](https://docs.nexustok.ai/ja/docs/api/ai-model/chat/create-message)
- ⚡ [Google Gemini](https://docs.nexustok.ai/ja/api/google-gemini-chat)
- 🔄 [Rerankモデル](https://docs.nexustok.ai/ja/docs/api/ai-model/rerank/create-rerank)（Cohere、Jina）

**インテリジェントルーティング:**
- ⚖️ チャネル重み付けランダム
- 🔄 失敗自動リトライ
- 🚦 ユーザーレベルモデルレート制限

**フォーマット変換:**
- 🔄 **OpenAI Compatible ⇄ Claude Messages**
- 🔄 **OpenAI Compatible → Google Gemini**
- 🔄 **Google Gemini → OpenAI Compatible** - テキストのみ、関数呼び出しはまだサポートされていません
- 🚧 **OpenAI Compatible ⇄ OpenAI Responses** - 開発中
- 🔄 **思考からコンテンツへの機能**

**Reasoning Effort サポート:**

<details>
<summary>詳細設定を表示</summary>

**OpenAIシリーズモデル:**
- `o3-mini-high` - 高思考努力
- `o3-mini-medium` - 中思考努力
- `o3-mini-low` - 低思考努力
- `gpt-5-high` - 高思考努力
- `gpt-5-medium` - 中思考努力
- `gpt-5-low` - 低思考努力

**Claude思考モデル:**
- `claude-3-7-sonnet-20250219-thinking` - 思考モードを有効にする

**Google Geminiシリーズモデル:**
- `gemini-2.5-flash-thinking` - 思考モードを有効にする
- `gemini-2.5-flash-nothinking` - 思考モードを無効にする
- `gemini-2.5-pro-thinking` - 思考モードを有効にする
- `gemini-2.5-pro-thinking-128` - 思考モードを有効にし、思考予算を128トークンに設定する
- Gemini モデル名の末尾に `-low` / `-medium` / `-high` を付けることで推論強度を直接指定できます（追加の思考予算サフィックスは不要です）。

</details>

## 🚢 デプロイ

> [!TIP]
> **最新のDockerイメージ:** `c1cadabob/nexustok:latest`
>
> **v0.2.6イメージ:** `c1cadabob/nexustok:v0.2.6`

### 📋 デプロイ要件

| コンポーネント | 要件 |
|------|------|
| **本番データベース** | PostgreSQL 15（アプリケーションは PostgreSQL ≥ 9.6 もサポート） |
| **本番キャッシュ** | Redis 7（Compose 内部ネットワーク） |
| **互換データベース** | MySQL ≥ 5.7.8、SQLite |
| **コンテナエンジン** | Docker / Docker Compose |
| **システムアーキテクチャ** | 64ビットのみ対応（amd64 / arm64）。32ビットシステムは非対応 |

> Compose が本番環境の推奨入口です。単一コンテナコマンドは SQLite／Redis なしの互換
> モードのみで、PostgreSQL と Redis は自動作成されません。既存の SQLite ファイルも
> PostgreSQL へ自動移行されません。

### ⚙️ 環境変数設定

<details>
<summary>一般的な環境変数設定</summary>

| 変数名 | 説明 | デフォルト値 |
|--------|------|--------|
| `SESSION_SECRET` | 認証署名シークレット。すべてのノードで同じ値が必要 | - |
| `SESSION_COOKIE_SECURE` | `false`/未設定ではローカル HTTP 開発プロキシ向けに refresh/logout の OriginGuard を無効化し、`true` では Secure Cookie と厳格な Origin 検証を有効化 | `false` |
| `SESSION_COOKIE_TRUSTED_URL` | Secure モードでは必須。refresh/logout を許可する完全一致の HTTPS Origin をカンマ区切りで指定。relay CORS 設定ではありません | - |
| `TRUSTED_PROXIES` | 未設定/空ではループバック、RFC 1918、IPv6 ULA を信頼して起動時に警告し、`none` ではすべて無効、明示的なプロキシ IP/CIDR リストは既定値を完全に置き換えます | `127.0.0.0/8, ::1, 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, fc00::/7` |
| `USER_SESSION_ACTIVE_LIMIT` | 1 ユーザーあたりの有効なログイン Session 上限 | `50` |
| `USER_SESSION_ISSUANCE_LIMIT` | カウント期間内に作成できる Session 数の上限（取り消し済みを含む） | `100` |
| `USER_SESSION_ISSUANCE_WINDOW_SECONDS` | Session 発行のカウント期間（秒）。取り消し済み Session の保持期間を超える場合は自動的に制限 | `86400` |
| `USER_SESSION_REVOKED_RETENTION_DAYS` | 監査と発行数計算のため取り消し済み Session を保持する日数 | `7` |
| `USER_SESSION_HOURLY_ALERT_THRESHOLD` | 1 時間あたりのグローバル Session 発行数の警告閾値。ログインは拒否しません | `5000` |
| `CRYPTO_SECRET` | キャッシュキー用 HMAC シークレット。Redis を共有するノードでは同じ実効値が必要 | デフォルトは `SESSION_SECRET` |
| `SQL_DSN** | データベース接続文字列 | - |
| `REDIS_CONN_STRING` | Redis接続文字列 | - |
| `STREAMING_TIMEOUT` | ストリーミング応答のタイムアウト時間（秒） | `300` |
| `STREAM_SCANNER_MAX_BUFFER_MB` | ストリームスキャナの1行あたりバッファ上限（MB）。4K画像など巨大なbase64 `data:` ペイロードを扱う場合は値を増加させてください | `64` |
| `MAX_REQUEST_BODY_MB` | リクエストボディ最大サイズ（MB、**解凍後**に計測。巨大リクエスト/zip bomb によるメモリ枯渇を防止）。超過時は `413` | `32` |
| `AZURE_DEFAULT_API_VERSION` | Azure APIバージョン | `2025-04-01-preview` |
| `ERROR_LOG_ENABLED` | エラーログスイッチ | `false` |
| `PYROSCOPE_URL` | Pyroscopeサーバーのアドレス | - |
| `PYROSCOPE_APP_NAME` | Pyroscopeアプリ名 | `NexusTok` |
| `PYROSCOPE_BASIC_AUTH_USER` | Pyroscope Basic Authユーザー | - |
| `PYROSCOPE_BASIC_AUTH_PASSWORD` | Pyroscope Basic Authパスワード | - |
| `PYROSCOPE_MUTEX_RATE` | Pyroscope mutexサンプリング率 | `5` |
| `PYROSCOPE_BLOCK_RATE` | Pyroscope blockサンプリング率 | `5` |
| `HOSTNAME` | Pyroscope用のホスト名タグ | `NexusTok` |

📖 **完全な設定:** [環境変数ドキュメント](https://docs.nexustok.ai/ja/docs/installation/config-maintenance/environment-variables)

</details>

### 🖥️ 単一マシンへのデプロイ（PostgreSQL + Redis）

64 ビット Linux ホストに Docker Engine、Docker Compose v2、Git、`curl` を準備します。
ファイアウォールは `3030` のみを開放し、HTTPS リバースプロキシを使う場合はプロキシ
用の `80/443` のみを開放します。PostgreSQL `5432` と Redis `6379` は Compose 内部
ネットワークだけで利用し、公開しないでください。

```bash
git clone https://github.com/c1cadaBob/NexusTok.git
cd NexusTok
bash scripts/deploy.sh
```

初回実行時、スクリプトは権限 `0600` の `.env` を作成し、ランダムな
`POSTGRES_PASSWORD` と `REDIS_PASSWORD` を生成します。既存の値は上書きしません。
`docker compose config` で検証してからイメージを取得し、PostgreSQL と Redis を起動します。
NexusTok の起動前に Compose ネットワーク上で実際のパスワード認証を確認し、その後 3
サービスのヘルスチェックを待機します。

```bash
docker compose ps
curl http://127.0.0.1:3030/api/status
curl http://127.0.0.1:3030/api/setup
```

状態 API が成功したら `http://サーバーアドレス:3030` を開き、セットアップウィザードで
初期化と管理者アカウント作成を行います。リバースプロキシは SSE、WebSocket、長時間
接続、`X-Forwarded-*` ヘッダーを転送してください。HTTPS では
`SESSION_COOKIE_SECURE=true` と、実際の HTTPS Origin を
`SESSION_COOKIE_TRUSTED_URL` に設定します。`TRUSTED_PROXIES` には信頼できるプロキシの
IP/CIDR だけを指定してください。

次の単一コンテナコマンドは互換モードのみです。

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
  c1cadabob/nexustok:v0.2.6
```

PostgreSQL と Redis は起動しません。更新前に `.env`、`/opt/nexustok/data`、
`/opt/nexustok/logs`、PostgreSQL named volume をバックアップしてください。

```bash
docker compose exec -T postgres pg_dump -U root nexustok > nexustok-$(date +%F).sql
cp .env /secure-backup/nexustok.env
docker compose pull
docker compose up -d
```

ロールバックでは直前に検証したバージョンに固定して `docker compose up -d` を実行します。
PostgreSQL named volume は削除しないでください。
SQLite は PostgreSQL に自動移行されないため、既存データはバックアップ後に別途検証済み
の移行を行う必要があります。

`.env` の `POSTGRES_PASSWORD` を変更しても、既存の PostgreSQL named volume 内のロール
パスワードは変更されません。ネットワーク認証に失敗した場合は以前の値へ戻すか、
論理バックアップ後にデータベース側のパスワードを別途同期してください。パスワードを
変更するためだけに本番 volume を削除しないでください。

よくある問題は、`3030` の競合、PostgreSQL/Redis の unhealthy 状態
（`docker compose logs postgres redis`）、空または変更された `.env` パスワード、
データディレクトリの権限、SELinux/AppArmor、amd64/arm64 の不一致、プロキシのタイム
アウトや SSE/WebSocket の Upgrade ヘッダー不足です。全新規デプロイのリセットでは、
バックアップを確認してから NexusTok のコンテナ、volume、
`/opt/nexustok/data`、`/opt/nexustok/logs` だけを削除してください。
`docker system prune -a`、`docker volume prune`、範囲未確認の
`docker compose down -v` は使わず、Komari など他のリソースを残してください。
Docker socket はホストの Docker 管理権限に相当するため、信頼できる管理者専用の環境で
のみマウントしてください。

トラブルシューティング：[FAQ](https://docs.nexustok.ai/ja/docs/support/faq)。

### 🌐 複数マシンへのデプロイ

各アプリケーションノードで内蔵 PostgreSQL と Redis を起動しないでください。内部
ネットワーク、アクセス制御、TLS で保護した共有 PostgreSQL と共有 Redis を用意します。
全ノードで `SQL_DSN`、`REDIS_CONN_STRING`、`SESSION_SECRET`、`CRYPTO_SECRET` を同じ
値にし、`NODE_NAME` はノードごとに一意にします。

主ノードは `NODE_TYPE` を設定せず、データベース移行とシステムタスクを担当します。
従属ノードは `NODE_TYPE=slave` を設定し、リクエスト処理のみを行います。全ノードの
時刻を同期し、ロードバランサーのヘルスチェックに `/api/status` を使用してください。
不健康なノードは作業前にロードバランサーから除外します。

```bash
docker run --name nexustok-node-1 -d --restart always \
  -p 3030:3030 \
  --env-file /etc/nexustok/node.env \
  -e NODE_NAME=node-1 \
  -v /opt/nexustok/data:/data \
  -v /opt/nexustok/logs:/app/logs \
  -v /var/run/docker.sock:/var/run/docker.sock \
  c1cadabob/nexustok:v0.2.6
```

`node.env` には少なくとも `SQL_DSN`、`REDIS_CONN_STRING`、`SESSION_SECRET`、
`CRYPTO_SECRET` を記載し、従属ノードには `NODE_TYPE=slave` を追加します。主ノードの
移行とヘルスチェック完了後、従属ノードを一台ずつ除外、更新、確認、再参加させ、最後
にメンテナンス時間帯で主ノードを更新します。ロールバック前にデータベース移行との
互換性を確認し、復元テスト済みの PostgreSQL バックアップを用意してください。

共有 Redis では Session、レート制限、キャッシュ制御面を共有できます。ノードごとに
独立した Redis を使うと伝播遅延とノード単位の制限になります。Redis を使わない場合は
Session がデータベースへフォールバックし、レート制限はプロセス単位です。Docker
socket、ノードのログ、`/data` は自動的に共有ストレージにはならないため、集中ログや
共有ファイルは別途設計してください。

### 🔄 チャネルリトライとキャッシュ

**リトライ設定:** `設定 → 運営設定 → 一般設定 → 失敗リトライ回数`

**キャッシュ設定:**
- `REDIS_CONN_STRING`：Redisキャッシュ（推奨）
- `MEMORY_CACHE_ENABLED`：メモリキャッシュ

## 📜 ライセンス

このプロジェクトは [GNU Affero General Public License v3.0 (AGPLv3)](./LICENSE) の下でライセンスされています。

本プロジェクトは、[One API](https://github.com/songquanpeng/one-api)（MITライセンス）をベースに開発されたオープンソースプロジェクトです。

お客様の組織のポリシーがAGPLv3ライセンスのソフトウェアの使用を許可していない場合、またはAGPLv3のオープンソース義務を回避したい場合は、こちらまでお問い合わせください：[support@c1cadabob.dev](mailto:support@c1cadabob.dev)

<div align="center">

### 💖 NexusTokをご利用いただきありがとうございます

このプロジェクトがあなたのお役に立てたなら、ぜひ ⭐️ スターをください！

**[問題フィードバック](https://github.com/c1cadabob/nexustok/issues)** • **[最新リリース](https://github.com/c1cadabob/nexustok/releases)**

<sub>❤️ で構築された c1cadaBob</sub>

</div>
