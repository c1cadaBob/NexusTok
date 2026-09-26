# AGENTS.md — Project Conventions for NexusTok

## 项目对话语言

**所有对话必须使用中文**。代码注释、文档、commit 信息、issue、PR 描述等均使用中文。

DO NOT send optional commentary

## Overview

This is an AI API gateway/proxy built with Go. It aggregates 40+ upstream AI providers (OpenAI, Claude, Gemini, Azure, AWS Bedrock, etc.) behind a unified API, with user management, billing, rate limiting, and an admin dashboard.

## Tech Stack

- **Backend**: Go 1.25.1 (see each module’s `go.mod`), Gin web framework, GORM v2 ORM
- **Frontend**: React 19, TypeScript, Rsbuild 2, TanStack Router/Query/Table, Zustand, Base UI, Tailwind CSS 4
- **Databases**: SQLite, MySQL, PostgreSQL for the primary database (all three must be supported); a separately configured log database also supports ClickHouse
- **Cache**: Redis (go-redis) + in-memory cache
- **Auth**: Browser sessions, API tokens and personal access tokens, JWT, WebAuthn/Passkeys, TOTP, OAuth/OIDC; Casbin authorization in `service/authz/`
- **Extensions**: JavaScript task plugins executed by Sobek; Electron desktop wrapper
- **Frontend package manager**: Bun (preferred over npm/yarn/pnpm)

## Architecture

- The Go gateway handles management APIs, upstream relay, billing, and background tasks across `router/`, `middleware/`, `controller/`, `service/`, `model/`, and `relay/`.
- `relaykit/` is an independent Go module for protocol DTOs and conversions; transport, authentication, database access, and billing stay in the host.
- JavaScript task plugins live in `plugins/tasks/`, run through `pkg/jsplugin/`, and integrate with host task polling and settlement.
- `web/` is the React frontend (see `web/AGENTS.md`); `electron/` is the desktop wrapper.

## 平台站点参考源项目

涉及 NewAPI 或 Sub2API 平台站点的认证、会话刷新、资源获取、密钥同步、模型能力、端点发现和管理端交互时，必须先核对以下本机参考源项目中的实际路由、请求参数、响应结构、权限要求和失败语义：

- Sub2API 源码：`/opt/project/sub2api-main`
- New API 源码：`/opt/project/new-api-main`
- all-api-hub 源码：`/opt/project/all-api-hub-main`

用户原始请求中的 New API 路径与 Sub2API 路径重复填写为 `/opt/project/sub2api-main`；本机实际存在的 New API 参考源为 `/opt/project/new-api-main`，以后以该路径为准。

不得只根据平台名称、README、外部说明或单个适配器方法推断完整能力。修改前应沿参考项目的真实路由、请求 DTO、响应 DTO、认证中间件和错误处理追踪端到端行为。参考项目中的凭据、测试账号、Cookie、Token、Refresh Token、环境变量和其它敏感运行数据不得复制到 NexusTok。

截至 2026-09-26，平台站点实现的代码事实入口包括 `service/platform_site_auth_flow.go`、
`service/upstream_site.go`、`service/upstream_site_adapters.go`、`model/platform_site_resources.go`
和 `controller/upstream_channel.go`。涉及认证流程、会话刷新、资源快照或资源管理接口时，
必须同时核对这些入口与上述三个参考源；资源失败时应保持最近成功快照，不能把权限不足、
安全验证或部分分页失败误判为密钥不存在。

## Internationalization (i18n)

### Backend (`i18n/`)
- Library: `nicksnyder/go-i18n/v2`
- Languages: en, zh

### Frontend (`web/src/i18n/`)
- Library: `i18next` + `react-i18next` + `i18next-browser-languagedetector`
- Languages: en (base), zh (fallback), zh-TW, fr, ru, ja, vi
- Translation files: `web/src/i18n/locales/{lang}.json` — flat JSON, keys are English source strings
- Usage: `useTranslation()` hook, call `t('English key')` in components
- CLI tools: `bun run i18n:sync` (from `web/`)

## Rules

### Common Code Quality

- New code should stay direct and readable. Prefer early returns, clear branches, and well-named local variables to deep nesting or layered control flow.
- Minimize nested function definitions. Use them only when required by a callback API or when keeping the closure local is clearly simpler than adding another symbol.
- Avoid adding package-level or module-level helper functions that have only one caller and do not express a stable business concept. Inline that logic at the call site instead.
- A separate function is appropriate when it represents reusable behavior, a required interface/framework callback, an exported API, a test fixture, or complex business logic that deserves direct tests.
- If a single-use helper is kept, its name must describe a durable domain concept rather than a mechanical step extracted only to shorten the caller.

### Authentication Security (OWASP Mandatory)

- Any implementation, modification, or review involving authentication-related flows MUST comply with the applicable requirements of the latest stable [OWASP Application Security Verification Standard (ASVS)](https://owasp.org/www-project-application-security-verification-standard/) and the relevant [OWASP Cheat Sheet Series](https://cheatsheetseries.owasp.org/). This applies to both backend and frontend changes, including registration, login/logout, password changes and recovery, email verification, MFA, WebAuthn/Passkeys, OAuth/OIDC, account linking/unlinking, sessions, JWTs, API credentials, and re-authentication for sensitive actions.
- Before changing these flows, read the applicable OWASP guidance, starting with the [Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html) and [Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html). Consult the password storage, forgot password, MFA, OAuth, and CSRF guidance when those mechanisms are involved. Identify the applicable controls before implementation; existing code is not a justification for retaining or introducing an insecure pattern.
- Enforce security controls on the server. Apply the relevant requirements for credential storage and transport, resistance to account enumeration and brute force, CSRF and replay protection, token/challenge expiry and single use where required, protocol-specific verification, session rotation and invalidation, and re-authentication for sensitive account changes. Frontend checks MUST NOT substitute for server-side enforcement, and recovery or alternative login paths MUST NOT bypass the required authentication assurance.
- Authentication audit events MUST exclude passwords, verification codes, recovery codes, private keys, and usable session or authentication tokens. Record enough non-secret context to investigate authentication failures and sensitive account changes.
- Verify affected security controls with focused regression tests, including applicable failure, expiry, replay, and bypass cases, following the existing backend/frontend test conventions. Record the OWASP references (including the ASVS version and requirement IDs when used), validation performed, and any unresolved gaps in the change summary or PR description. Do not claim compliance or completion while an applicable security requirement remains unmet or unverified.

### Backend Rules

**Modern Go conventions:** Apply these conventions to new or modified Go code, including tests and `relaykit/`, when they preserve behavior and improve readability. Use the Go version declared in the relevant module's `go.mod` as the compatibility baseline.

- Use `any` instead of `interface{}`, including map values, slice elements, parameters, and return types.
- For fixed-count loops, prefer `for i := range n`, or `for range n` when the index is unused. For slice indices, prefer `for i := range items`. Keep conventional loops when the bound changes during iteration or the loop needs a different start or step.
- When split results are only traversed once without indexing or reuse, prefer `strings.SplitSeq` or `bytes.SplitSeq` over allocating a slice with `Split`.
- Use `strings.Cut` when splitting at the first separator, and `strings.CutPrefix` / `strings.CutSuffix` when checking and removing a prefix or suffix. Avoid separate searches and manual slicing for the same operation.
- Use `slices.Contains` / `slices.ContainsFunc` for membership checks and `slices.Sort` for natural ordering of ordered element types instead of equivalent hand-written loops or sort callbacks.
- Use `maps.Copy` for shallow map copies and merges. Initialize the destination as needed, and preserve nil-versus-empty behavior and the order in which later values overwrite earlier ones. It does not replace a deep copy.
- Use built-in `min` / `max` for simple bounds instead of equivalent conditional assignments. Preserve numeric semantics; these functions do not prevent overflow in their arguments or replace billing validation and safe quota conversion.
- Use `strings.Builder` for repeated string concatenation in loops; retain direct concatenation for simple fixed expressions.
- Use `reflect.TypeFor[T]()` when the type is known statically, and `reflect.Pointer` instead of `reflect.Ptr`. Keep `reflect.TypeOf` when the dynamic type of a value is required.
- Prefer `sync.WaitGroup.Go` for the standard `Add(1)` / goroutine / deferred `Done()` pattern when its lifecycle and panic contract apply. Preserve existing recovery behavior; the function passed to `Go` must not panic.
- Remove redundant loop-variable copies such as `tc := tc` when they exist only for pre-Go-1.22 closure capture. Retain copies needed for actual snapshot semantics or variables assigned outside the loop.
- Remove ineffective `omitempty` tags on non-pointer struct fields only after confirming the active JSON encoder preserves the same output. Do not change field types or omission behavior as part of a style cleanup; optional relay scalar fields must still follow the pointer rules below.
- Format modified Go files with `gofmt` and remove unused imports after these changes.

**relaykit module independence:** The `relaykit/` Go module MUST remain independently buildable.

- Code under `relaykit/` MUST NOT import or depend on packages from the root `NexusTok` module, or rely on root-only configuration, generated files, or workspace wiring.
- Any change affecting `relaykit/` or its public APIs MUST be verified with `cd relaykit && GOWORK=off go build ./...`; a successful root-module build is not sufficient.

**JSON package:** In the root Go module, all JSON marshal/unmarshal operations MUST use the wrapper functions in `common/json.go`:

- `common.Marshal(v any) ([]byte, error)`
- `common.Unmarshal(data []byte, v any) error`
- `common.UnmarshalJsonStr(data string, v any) error`
- `common.DecodeJson(reader io.Reader, v any) error`
- `common.GetJsonType(data json.RawMessage) string`

Do NOT directly import or call `encoding/json` in business code. `json.RawMessage`, `json.Number`, and other type definitions from `encoding/json` may still be referenced as types, but actual marshal/unmarshal calls must go through `common.*`.

Inside `relaykit/`, use `kitutil.*` from `relaykit/relayconvert/kitutil/json.go`, never host `common`. Direct encoder calls belong only in codec implementations.

**Database compatibility:** All database code MUST work with SQLite, MySQL >= 5.7.8, and PostgreSQL >= 9.6 simultaneously.

- Any change that can affect database behavior MUST be verified before the work is considered complete. This includes ORM/database-driver dependency changes, connection/DSN/protocol or prepared-statement configuration, models and GORM tags, migrations and `AutoMigrate`, constraints and indexes, `Scanner`/`Valuer`/serializer behavior, raw SQL, transactions, and row locking.
- Required database verification MUST exercise real SQLite, MySQL, and PostgreSQL instances. Unit tests, mocks, a successful build, code inspection, or testing only one dialect are not substitutes. Use at least one supported version of each engine; changes that depend on version-specific behavior must also cover the minimum supported version.
- Treat GORM core and its database dialect/driver packages as a compatible version set. Any change to one of them requires checking upstream compatibility and running the complete three-database verification matrix; do not upgrade only the core package and infer that existing drivers remain compatible.
- Schema or migration changes MUST be tested both on a fresh database and by upgrading a representative database created by the latest released version. Run startup/migration at least twice to prove idempotency, and verify that existing data, indexes, constraints, and uniqueness guarantees are preserved. Cover the separately configured log database when the affected path is shared with or used by it.
- Record the exact database versions, commands, and results in the final handoff or pull request. If any required database verification cannot be run, report the blocker explicitly and do not claim the change is database-compatible or complete.
- Prefer GORM methods (`Create`, `Find`, `Where`, `Updates`, etc.) over raw SQL.
- Let GORM handle primary key generation; do not use `AUTO_INCREMENT` or `SERIAL` directly.
- Standard `SELECT ... FOR UPDATE` row locks built with GORM query methods in `model/` MUST use `lockForUpdate(tx)`. Do not use the legacy GORM v1 pattern `tx.Set("gorm:query_option", "FOR UPDATE")`, because GORM v2 silently ignores it and no lock is acquired. Do not duplicate `clause.Locking{Strength: "UPDATE"}` at call sites; the shared helper emits `FOR UPDATE` for MySQL/PostgreSQL and skips it for SQLite, where the syntax is unsupported. Dialect-specific locking with different semantics (for example, a MySQL next-key/gap lock) may use raw SQL only behind explicit database-type branches with valid fallbacks for every supported database.
- When raw SQL is unavoidable, account for dialect differences:
  - PostgreSQL uses `"column"` quoting, while MySQL/SQLite use `` `column` ``.
  - Use `commonGroupCol`, `commonKeyCol` from `model/main.go` for reserved-word columns like `group` and `key`.
  - Use `commonTrueVal`/`commonFalseVal` for boolean values.
  - Use `common.UsingMainDatabase(...)` for primary database branches and `common.UsingLogDatabase(...)` for log database branches.
- Do not use database-specific features without cross-DB fallback, including MySQL-only functions, PostgreSQL-only operators, SQLite-unsupported `ALTER COLUMN`, or database-specific JSON column types without a `TEXT` fallback.
- Migrations must work on all three databases. For SQLite, use `ALTER TABLE ... ADD COLUMN` instead of `ALTER COLUMN` (see `model/main.go` for patterns).
- Avoid GORM boolean default tags such as `gorm:"default:true"` when the default is a business rule already enforced by code. MySQL and PostgreSQL can normalize boolean defaults differently, causing GORM `AutoMigrate` to repeatedly issue `ALTER TABLE` on restart. Prefer setting these defaults in request/model normalization, hooks, constructors, or service logic; do not replace `default:true` with `default:1` unless the behavior is verified across SQLite, MySQL, and PostgreSQL.

**Relay and provider behavior:**

- When implementing a new channel, confirm whether the provider supports `StreamOptions`; if supported, add the channel to `streamSupportedChannels`.
- For request structs parsed from client JSON and re-marshaled to upstream providers, optional scalar fields MUST use pointer types with `omitempty` (for example, `*int`, `*uint`, `*float64`, `*bool`).
- Preserve explicit zero values in upstream relay request DTOs: absent client JSON fields must become `nil` and be omitted, while explicit `0`, `0.0`, or `false` values must remain non-`nil` and be sent upstream.
- Avoid non-pointer scalars with `omitempty` for optional request parameters, because zero values will be silently dropped during marshal.

**Billing expression system:** When working on tiered/dynamic billing (expression-based pricing), MUST read `pkg/billingexpr/expr.md` first. It documents the design philosophy, expression language, full architecture, token normalization rules, quota conversion, and expression versioning. All billing expression changes must follow that document.

**Built-in model pricing:** New built-in model prices MUST be defined as self-contained billing expressions in `setting/billing_setting/builtin_billing.go`, using real USD per million tokens. Do not add new built-in prices to the legacy model/completion/cache ratio tables. Preserve explicit administrator pricing overrides. Existing legacy prices are migrated only when explicitly requested. Verify published prices and cover applicable context-length thresholds and cache categories.

**Billing safety invariants:** Quota/billing code MUST never produce a negative charge (a credit) from arithmetic overflow or unvalidated input. Apply defense in depth:

- Every user-controlled quantity that becomes a billing multiplier (image `n`, video `seconds`/`duration`, resolution/quality ratios, batch counts) MUST be bounded before it reaches quota calculation. Reject out-of-range values at request validation with a 400. Existing bounds: `dto.MaxImageN` for image generation count, `relaycommon.MaxTaskDurationSeconds` for task video duration, `maxTokensLimit` (`relay/helper/valid_request.go`) for `max_tokens`-family fields on every relay format (OpenAI, Claude, Gemini, Responses). Reuse these constants instead of introducing new ad hoc limits for the same concepts. When adding a new relay format or request DTO, bound its max-tokens and count fields in its validator from day one.
- Watch for validation bypass paths: passthrough fields (e.g. `Extra["parameters"]`), task `metadata` maps, and multipart form fields can carry the same quantities around the standard DTO validation. Any adaptor that reads a multiplier from such a path must enforce the same bound (or clamp) locally.
- Durations parsed from media metadata are user/upstream-controlled too: audio file headers (transcription token counting, TTS response duration) and upstream deduction numbers (e.g. Kling `FinalUnitDeduction`) can claim absurd values. Convert them with saturation before they become token counts.
- Never convert a computed quota or token count to `int` with a bare cast like `int(float64(quota) * ratio)`, `int(math.Round(...))` on unbounded input, or `int(decimal.IntPart())`. All quota rounding/conversion is centralized in `common/quota_math.go`; use those helpers: `common.QuotaFromFloat` (truncating) for float products, `common.QuotaRound` (half-away-from-zero) where rounding is intended, and `common.QuotaFromDecimal` for decimal products. `billingexpr.QuotaRound` delegates to `common.QuotaRound`. Do not reintroduce local conversion helpers or bare casts. Single-request saturation stays at the int32 boundary so batch accumulation cannot approach 64-bit wraparound; wallet/top-up conversion uses `common.WalletQuotaFromDecimalStrict` with the JavaScript-safe `common.MaxWalletQuota` boundary. Every clamp/NaN fallback is logged via `common.SysError`.
- Saturation events are also audited: each helper has a `*Checked` variant (`common.QuotaFromFloatChecked` / `QuotaRoundChecked` / `QuotaFromDecimalChecked`) that additionally returns a `*common.QuotaClamp` when clamping occurred. Billing paths that compute a charge capture that clamp onto `relayInfo.QuotaClamp` (or thread it into task settlement) and, right before writing the consume/task log, call `attachQuotaSaturation` (in `service/log_info_generate.go`) which nests the marker under the log's `other.admin_info.quota_saturation` and emits a request-correlated `logger.LogWarn`. Nesting under `admin_info` makes it admin-only for free (non-admin log views strip `admin_info`). When adding a new billing path, use the `*Checked` variant and surface the clamp the same way so the anomaly stays auditable in both the admin log UI and backend logs.
- Multiplier maps go through `types.PriceData.AddOtherRatio`, which rejects non-positive, NaN, and +Inf ratios. Do not write to `PriceData.OtherRatios` directly, and do not weaken these guards.
- Pre-consume (预扣费) and settle (结算/差额) must both be safe: a saturated oversized quota must fail pre-consume with insufficient-quota, never silently wrap. When adding a new billing path (new relay format, new task platform, new adjustment hook), trace the full chain — validation → EstimateBilling/OtherRatios → quota conversion → pre-consume → settle/refund — and confirm each step preserves these invariants.
- Fields parsed into unsigned types (`*uint`) accept huge positive JSON numbers (e.g. `18446744073686646784`, a wrapped negative); a `>= 0` check is not sufficient, an upper bound is mandatory.
- Regression tests for these invariants belong with the boundary they protect (request validators, converter helpers). See `relay/helper/openai_image_request_test.go`, `relay/common/relay_utils_test.go`, and `common/quota_math_test.go` for the expected style.

**Backend test quality:** Backend tests must protect real behavior, API contracts, billing/accounting invariants, data compatibility, or regression paths.

- **Do not scatter tests for a small change:** For a focused feature or fix, extend an existing suitable test file first. If a new test file is necessary, add at most one and consolidate the key regression cases there. MUST NOT create separate test files for the same small feature across `controller/`, `service/`, `setting/`, or other layers merely because its call chain crosses those layers. Do not repeat fixtures and assertions at each layer. Keep the cases compact and focused on observable behavior; the number of production files touched is not a reason to add more test files.
- Do not add tests that only improve coverage numbers, prove that code happens to run, or lock in implementation details without a user-visible or cross-module contract.
- Avoid fake fuzz/stress/smoke/performance tests built from random inputs, large loop counts, sleeps, timing comparisons, or log-only assertions.
- Avoid duplicate tests that exercise the same branch with different names but no new invariant.
- Avoid tests that force incorrect provider/protocol semantics into production code.
- Avoid tests that assert private constants, select-field lists, helper internals, or file layout when observable behavior is already covered elsewhere.
- Prefer deterministic table tests with explicit inputs and exact expected outputs.
- When tests need database, request context, user group, settings, or cache state, initialize that state explicitly inside the test fixture.
- New or substantially rewritten Go backend tests MUST use `github.com/stretchr/testify/require` for setup and fatal assertions, and `github.com/stretchr/testify/assert` for non-fatal value checks.
- Avoid hand-written assertion helpers unless they encode a reusable project-specific invariant.
- When cleaning tests, preserve meaningful regression coverage. If a deleted test covered a real contract indirectly, replace it with a smaller test that asserts that contract directly.

**Documentation files:** Do NOT add new files under `docs/` or any of its subdirectories unless the user explicitly requests it.

- 每当优化或调整密钥调用方式、渠道调度规则、测试入口、模型获取行为、密钥可用模型限制或日志可观测规则时，必须同步更新并校准 [`docs/key-routing-strategy.md`](docs/key-routing-strategy.md)。
- 文档更新必须与代码变更在同一个功能提交中完成；提交前核对接口参数、候选过滤条件、优先级/权重公式、失败回退行为和管理员可见字段，确保文档描述与当前实现一致。

### Frontend Rules

- **Reuse existing UI components first (mandatory):** Before implementing or changing frontend UI, read `web/AGENTS.md` and the project `shadcn-ui` skill, search `web/src/components/` and the relevant feature for existing components, and read matching implementations and call sites. Do not start from custom markup or registry installation without checking the repository first.
- Prefer the project's shared business components over lower-level UI primitives when they cover the use case. Evaluate existing props, composition, and a compatible extension before introducing a replacement. Importing `Button` or `AlertDialog` does not satisfy this rule if the same behavior is already provided by a shared component such as `CopyButton` or `ConfirmDialog`.
- New implementations of common UI behavior require a concrete capability gap: identify the existing candidates and explain why reuse, composition, or a compatible extension is unsuitable in the change summary or PR description. Different text, dimensions, colors, or feature location alone do not justify duplication. Feature components may compose shared components with business data and actions. Follow the reuse workflow and component entry points in `web/AGENTS.md`; generic library or registry guidance does not override this project-specific priority.
- Use `bun` as the preferred package manager and script runner for the frontend (`web/`):
  - `bun install` for dependency installation
  - `bun run dev` for development server
  - `bun run build` for production build
  - `bun run i18n:*` for i18n tooling
- Frontend UI text must support i18n with `i18next`/`react-i18next`. Use flat JSON locale files in `web/src/i18n/locales/{lang}.json`, with English source strings as keys.
- In React components, use `useTranslation()` and call `t('English key')` for user-facing text.
- Follow `web/AGENTS.md` for detailed frontend conventions, including TypeScript, component structure, styling, accessibility, testing, and build checks.

### Project Governance

**版本控制工作流：**

- 每完成一个可独立验收的子功能，必须立即将该子功能涉及的文件执行 `git add` 暂存，并在继续实现下一个子功能前确认暂存内容仅包含本次子功能。
- 完成一个完整功能后，必须创建中文 commit 保存完整变更，并将该 commit 推送到当前分支对应的远端，确保代码在远端持久保存。
- 提交前必须检查 `git diff --cached`、测试结果和工作区状态；不得将密码、Cookie、Token、API Key、临时文件、构建产物或无关修改提交到仓库。
- 推送失败时必须保留本地 commit，明确报告失败原因和待执行的远端保存步骤；不得通过删除 commit 或覆盖其他人的提交来规避失败。

**功能复用原则：**

- 进行功能开发前，必须先检索并理解项目中已经实现的相同或相近功能，优先复用现有组件、服务、接口、数据结构、校验逻辑、权限控制和测试工具。
- 已有实现能够满足需求时不得重复创建；需要差异化行为时，优先通过扩展现有实现或组合既有能力完成。
- 只有在确认现有能力无法满足需求时才新增实现，并在提交说明或代码评审说明中简要记录复用评估结果和新增实现的必要性。

**功能原理文档同步维护：**

- 修改任何已有功能前，必须先检索 [`docs/architecture/README.md`](docs/architecture/README.md) 和对应的架构/详细文档，确认当前代码事实、接口入口和已知偏差。
- 修改路由、鉴权、权限、渠道、密钥、模型获取、请求转换、计费、额度、任务、插件、缓存、数据库、日志、配置或前端用户流程后，必须在同一功能变更中同步更新所有受影响的架构文档和专项详细文档。
- 新增渠道、Endpoint、Relay Format、配置项、数据库模型、后台任务、插件协议或公共业务规则时，必须同时更新对应功能说明和 [`docs/architecture/provider-capability-matrix.md`](docs/architecture/provider-capability-matrix.md)；不适用时须在变更说明中明确原因。
- 若代码行为与既有文档不一致，必须在同一功能变更中校正文档，并在 [`docs/architecture/implementation-deviations.md`](docs/architecture/implementation-deviations.md) 新增、更新或关闭对应偏差记录。
- 文档以当前实际代码为准，明确区分“已实现”“未实现”“设计目标”“待核查”；不得根据渠道名称、外部说明或单个 Adaptor 方法推断完整供应商能力。
- 每次文档更新必须写入实际变更日期，并用“变更前/变更后”记录具体行为差异，不得只写笼统的“更新说明”。
- 文档更新必须与对应代码变更处于同一个功能提交中。仅文档变更也须按可独立验收子功能暂存、检查，并纳入中文 commit。
- 提交前必须核对接口参数、路由挂载位置、中间件顺序、候选过滤条件、优先级/权重、失败回退、管理员可见字段、计费来源和日志字段，确保文档与实现一致。
- 只修改文档而不修改运行代码时，也必须静态核对链接、路径、代码入口、能力矩阵覆盖和偏差记录；无需因文档修改运行完整业务测试矩阵。

**Protected project information:** The following project-related information is strictly protected and MUST NOT be modified, deleted, replaced, or removed under any circumstances:

**Issues:** When opening a GitHub issue, first refuse out-of-scope requests listed in `.agents/github/ISSUE.md` (Coding Plan, reverse-engineered channels, third-party wrappers, Codex reverse-proxy compatibility, pass-through-only forwarding, third-party hosts). Tell the user and do not file. Then search https://docs.nexustok.ai/ , https://deepwiki.com/c1cadaBob/NexusTok , the README, and the code. If this is a usage, configuration, or integration question, answer the user from that material and do not file. Otherwise fill `.agents/github/ISSUE.md` as the entire body. If actual behavior, impact, frequency, evidence that the problem is in NexusTok, or the applicable relay/billing/frontend/deployment items are missing, ask the user those questions and wait. Do not invent them. Do not tell the user to confirm a template. Do not use GitHub issue forms.

**Pull requests:** When creating a pull request:

- First compare the current git user (`git config user.name` / `git config user.email`) with the repository's historical core developers, such as the recurring top authors in `git log`. Do not change git config.
- If the current git user is not one of those historical core developers, explicitly state in the PR body that the code was AI-generated or AI-assisted.
- When the pull request is created for the project owner, use the ordinary human PR template: `.github/PULL_REQUEST_TEMPLATE.md` for Chinese requests or `.github/PULL_REQUEST_TEMPLATE/en.md` for English requests. Project-owner pull requests MUST NOT use `.agents/github/PR.md` unless the owner explicitly asks for it.
- For all other agent-created pull requests, fill `.agents/github/PR.md` as the entire PR body. Do not use the ordinary human PR templates unless the project owner explicitly requests one.
