# NexusTok Electron 桌面端

`electron/` 目录提供 NexusTok 的桌面端包装层。桌面端会启动本地 NexusTok Go 后端，并通过 Electron 窗口加载本地 Web UI；生产包使用 Go 二进制内嵌的 `web/default/dist` 与 `web/classic/dist`，开发模式则连接本地前端开发服务器。

## 目录职责

| 文件 | 说明 |
|------|------|
| `main.js` | Electron 主进程，负责启动/检测后端、创建窗口、托盘菜单和错误日志。 |
| `preload.js` | 暴露最小桌面端上下文，例如 Electron 版本、平台和数据目录。 |
| `build.sh` | 一键构建默认前端、经典前端、Go 二进制和 Electron 安装包。 |
| `package.json` | Electron 依赖、运行脚本和 `electron-builder` 平台打包配置。 |
| `icon.png`、`tray-*` | 应用图标和系统托盘图标。 |

## 开发模式

开发模式不会由 Electron 主进程启动 Go 服务。你需要先在仓库根目录启动后端和默认前端开发服务器：

```bash
make dev-api
make dev-web
```

等默认前端可以通过 `http://localhost:3001` 访问后，再启动 Electron：

```bash
cd electron
npm ci
npm run dev-app
```

开发模式行为：

1. Electron 检查 `127.0.0.1:3001` 是否可用。
2. 窗口加载 `http://127.0.0.1:3001`。
3. DevTools 自动打开。
4. 系统托盘菜单可隐藏、显示或退出应用。

如果你不使用 Makefile，也可以手动启动：

```bash
# 终端 1：后端
go run main.go

# 终端 2：默认前端
cd web/default
bun install
bun run dev -- --host 0.0.0.0 --port 3001

# 终端 3：Electron
cd electron
npm ci
npm run dev-app
```

## 生产构建

推荐使用 `build.sh`，它会按 NexusTok 当前目录结构完成完整构建：

```bash
cd electron
./build.sh
```

脚本会依次执行：

1. 在 `web/default` 中执行 `bun install` 与 `bun run build`。
2. 在 `web/classic` 中执行 `bun install` 与 `bun run build`。
3. 在仓库根目录构建 `nexustok` 或 `nexustok.exe`。
4. 在 `electron/` 中安装 Electron 依赖并调用平台打包脚本。

也可以在确认前端 dist 与 Go 二进制都已准备好后，手动执行平台构建：

```bash
cd electron
npm ci
npm run build:mac
npm run build:win
npm run build:linux
```

打包输出位于 `electron/dist/`：

| 平台 | 输出类型 |
|------|----------|
| macOS | `.dmg` 与 `.zip` |
| Windows | NSIS 安装器与 portable exe |
| Linux | `.AppImage` 与 `.deb` |

## 运行时数据目录

生产模式下，Electron 主进程会为 Go 后端设置 `SQLITE_PATH`，数据库文件名为 `nexustok.db`。目录来自 Electron 的 `app.getPath('userData')`：

| 平台 | 典型位置 |
|------|----------|
| macOS | `~/Library/Application Support/NexusTok/data/` |
| Windows | `%APPDATA%/NexusTok/data/` |
| Linux | `~/.config/NexusTok/data/` |

备份桌面端数据时，复制对应 `data/` 目录即可。开发模式不会自动启动 Go 后端，也不会强制改写你手动启动后端的数据库路径。

## 许可证资源

`electron-builder` 会把仓库根目录的 `LICENSE`、`NOTICE`、`THIRD-PARTY-LICENSES.md` 以及 Electron 自带许可证文件打进安装包。新增桌面端依赖或更改构建资源时，需要同步检查这些资源路径，避免分发包缺少许可证声明。

## 常见问题

### 启动后提示无法连接前端开发服务器

确认默认前端开发服务器正在监听 `3001`：

```bash
make dev-web
```

或者手动执行：

```bash
cd web/default
bun run dev -- --host 0.0.0.0 --port 3001
```

### 启动后提示端口 3030 被占用

Electron 生产模式会启动内置 Go 后端并监听 `3030`。如果本机已经运行另一个 NexusTok 实例或本地开发后端，请先退出该进程。

### 打包时提示找不到前端资源

Go 二进制构建前必须存在：

```text
web/default/dist/index.html
web/classic/dist/index.html
```

直接运行 `electron/build.sh` 会自动生成这两套资源。手动打包时需要先分别执行默认前端和经典前端构建。

### 打包时提示找不到二进制

平台构建需要仓库根目录存在对应二进制：

| 平台 | 文件 |
|------|------|
| macOS/Linux | `nexustok` |
| Windows | `nexustok.exe` |

`electron/build.sh` 会自动生成。手动执行 `npm run build:*` 前需要先在仓库根目录运行对应的 `go build`。
