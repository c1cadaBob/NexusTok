<div align="center">

![nexustok](/web/default/public/logo.png)

# NexusTok

🍥 **次世代 LLM ゲートウェイと AI アセット管理システム**

<p align="center">
  <a href="./README.zh_CN.md">简体中文</a> |
  <a href="./README.en.md">English</a> |
  <a href="./README.zh_TW.md">繁體中文</a> |
  <a href="./README.fr.md">Français</a> |
  <strong>日本語</strong>
</p>

<p align="center">
  <a href="#-クイックスタート">クイックスタート</a> •
  <a href="#-主な機能">主な機能</a> •
  <a href="#-デプロイ">デプロイ</a> •
  <a href="#-ドキュメント">ドキュメント</a> •
  <a href="#-ライセンス">ライセンス</a>
</p>

</div>

## 📝 プロジェクト説明

> [!IMPORTANT]
> - NexusTok は、合法的に許可された AI API ゲートウェイ、組織内認証、複数プロバイダーのルーティング、利用分析、コスト計算、課金、プライベートデプロイのみを目的としています。
> - 利用者は、上流の API Key、アカウント、モデルサービス、インターフェース権限を合法的に取得し、上流の利用規約および適用される法令を遵守する必要があります。
> - 生成 AI サービスを公衆に提供する場合、利用者は管轄区域で求められる届出、許認可、コンテンツ安全、本人確認、ログ保存、税務、決済、上流認可などの義務を自ら履行する必要があります。

NexusTok は OpenAI 互換 API、Claude、Gemini、Azure、AWS Bedrock などの上流 AI サービスを統一 API の下に集約します。アカウントプール、クォータと課金、モデル価格、SystemTask の可観測性、トラフィック分析、管理コンソールを備え、自社運用の AI コントロールプレーンとして利用できます。

---

## ✨ 主な機能

| 領域 | 機能 |
|------|------|
| プロバイダーゲートウェイ | 40 以上の上流 AI プロバイダーを統合し、OpenAI 互換、Claude、Gemini、Azure、AWS Bedrock、Responses、タスク系ルートに対応します。 |
| アカウントプール | ネイティブな認証情報プール、アカウントグループ、OAuth/デバイス認可、ヘルスチェック、利用ログ、状態監査、チャンネル単位のプール連携。 |
| 課金とクォータ | Token/クォータ計算、モデル価格グループ、動的課金式、サブスクリプション、ウォレット残高、引換コード、飽和保護付きクォータ計算。 |
| ガバナンス | JWT、OAuth、WebAuthn/Passkeys、管理者権限 catalog、ルート単位の Authz enforcement、機密フィールド保護、管理操作監査ログ。 |
| 運用 | SystemTask runner、ノード間タスクロック、システムインスタンス heartbeat、ログ清理、チャンネルテスト、上流モデル同期、アカウントプール検査、非同期タスク polling。 |
| 分析 | 概要ダッシュボード、モデル/ユーザー利用グラフ、Dashboard Flow Sankey、利用ログ、ログイン監査、クォータ飽和の可視化、レコードエクスポート。 |
| 決済 | EPay、Stripe、Creem、Waffo、Waffo Pancake 設定、catalog/pair 連携、ホスト型 checkout 準備、サブスクリプション商品連携、webhook の冪等 top-up。 |
| フロントエンド | React 19 のデフォルトコンソール、classic 互換テーマ、モバイル向けログ/チャンネル表示、安全なリッチコンテンツ描画、Playground 改善、en/zh/fr/ja/ru/vi i18n。 |
| デプロイ | SQLite、MySQL >= 5.7.8、PostgreSQL >= 9.6、Redis またはメモリキャッシュ、Docker Compose、Go バイナリ、hot-reload 開発環境、Electron パッケージング。 |

---

## 🚀 クイックスタート

### Docker Compose（推奨）

```bash
git clone https://github.com/c1cada/NexusTok.git
cd NexusTok

# 本番利用前に docker-compose.yml を確認してください。
nano docker-compose.yml

docker-compose up -d
```

起動後、`http://localhost:3000` を開き、初期化ウィザードを完了してください。

<details>
<summary><strong>Docker コマンド</strong></summary>

```bash
docker pull c1cadabob/nexustok:latest

docker run --name nexustok -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  c1cadabob/nexustok:latest
```

`-v ./data:/data` は SQLite と実行時データをローカルの `data` ディレクトリに保存します。本番環境では絶対パスの利用を推奨します。

</details>

---

## 🚢 デプロイ

| コンポーネント | 要件 |
|----------------|------|
| ローカル DB | `/data` ボリュームを永続化した SQLite |
| リモート DB | MySQL >= 5.7.8 または PostgreSQL >= 9.6 |
| キャッシュ | Redis 推奨。小規模環境ではメモリキャッシュも利用可能 |
| 実行方式 | Docker / Docker Compose または Go バイナリ |

よく使う環境変数：

| 変数 | 説明 |
|------|------|
| `SESSION_SECRET` | 複数インスタンスでセッションを安定させるために必須です。 |
| `CRYPTO_SECRET` | Redis または共有暗号化データを有効にする場合に必須です。 |
| `SQL_DSN` | MySQL または PostgreSQL の接続文字列。空の場合は SQLite を使用します。 |
| `REDIS_CONN_STRING` | Redis 接続文字列。 |
| `MAX_REQUEST_BODY_MB` | 展開後のリクエストボディサイズ上限。 |
| `STREAMING_TIMEOUT` | ストリーミングタイムアウト（秒）。 |
| `STREAM_SCANNER_MAX_BUFFER_MB` | 大きなストリーミング断片の最大スキャナバッファ。 |

本番デプロイ、hot-reload 開発、バイナリデプロイ、テーマ切替、トラブルシューティングは [デプロイとメンテナンス](./docs/installation/deployment.md) を参照してください。

---

## 📚 ドキュメント

| トピック | リンク |
|----------|--------|
| デプロイとメンテナンス | [docs/installation/deployment.md](./docs/installation/deployment.md) |
| BT/Baota パネルインストール | [docs/installation/BT.md](./docs/installation/BT.md) |
| チャンネル高度設定 | [docs/configuration/channel-other-settings.md](./docs/configuration/channel-other-settings.md) |
| Backend OpenAPI | [docs/openapi/api.json](./docs/openapi/api.json) |
| Relay OpenAPI | [docs/openapi/relay.json](./docs/openapi/relay.json) |
| Electron デスクトップ | [electron/README.md](./electron/README.md) |
| Issue | [GitHub Issues](https://github.com/c1cada/NexusTok/issues) |
| リリース | [GitHub Releases](https://github.com/c1cada/NexusTok/releases) |

---

## 📜 ライセンス

NexusTok は [GNU Affero General Public License v3.0](./LICENSE) の下で公開されています。

本プロジェクトは MIT ライセンスの [One API](https://github.com/songquanpeng/one-api) を基に二次開発されています。

組織ポリシー上 AGPLv3 ソフトウェアを利用できない場合、または商用ライセンスが必要な場合は、[support@c1cada.dev](mailto:support@c1cada.dev) までご連絡ください。

---

<div align="center">

### NexusTok をご利用いただきありがとうございます

**[Repository](https://github.com/c1cada/NexusTok)** • **[Issues](https://github.com/c1cada/NexusTok/issues)** • **[Releases](https://github.com/c1cada/NexusTok/releases)**

<sub>c1cada とコントリビューターによって構築されています。</sub>

</div>
