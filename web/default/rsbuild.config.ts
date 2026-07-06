import path from 'path'
import { fileURLToPath } from 'url'
import { defineConfig, loadEnv } from '@rsbuild/core'
import { pluginReact } from '@rsbuild/plugin-react'
import { tanstackRouter } from '@tanstack/router-plugin/rspack'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

export default defineConfig(({ envMode }) => {
  const env = loadEnv({ mode: envMode, prefixes: ['VITE_'] })
  const serverUrl =
    process.env.VITE_REACT_APP_SERVER_URL ||
    env.rawPublicVars.VITE_REACT_APP_SERVER_URL ||
    'http://localhost:3000'

  const isProd = envMode === 'production'
  const isHotReloadBuild = process.env.NEXUSTOK_HOT_RELOAD === 'true'
  const devProxy = Object.fromEntries(
    (['/api', '/mj', '/pg'] as const).map((key) => [
      key,
      { target: serverUrl, changeOrigin: true },
    ]),
  ) as Record<string, { target: string; changeOrigin: boolean }>

  return {
    plugins: [pluginReact()],
    // Rsbuild 2: replaces deprecated `performance.chunkSplit` (RSPack 2 aligned)
    splitChunks: {
      preset: 'default',
      cacheGroups: {
        'vendor-react': {
          test: /node_modules[\\/](react|react-dom)[\\/]/,
          name: 'vendor-react',
          chunks: 'all',
          priority: 0,
          enforce: true,
        },
        'vendor-ui-primitives': {
          test: /node_modules[\\/](@base-ui|@radix-ui)[\\/]/,
          name: 'vendor-ui-primitives',
          chunks: 'all',
          priority: 0,
          enforce: true,
        },
        'vendor-tanstack': {
          test: /node_modules[\\/]@tanstack[\\/]/,
          name: 'vendor-tanstack',
          chunks: 'all',
          priority: 0,
          enforce: true,
        },
      },
    },
    source: {
      entry: {
        index: './src/main.tsx',
      },
    },
    resolve: {
      alias: {
        '@': path.resolve(__dirname, './src'),
      },
    },
    html: {
      template: './index.html',
    },
    server: {
      host: '0.0.0.0',
      proxy: devProxy,
    },
    output: {
      // Production optimizations
      minify: isProd,
      target: 'web',
      distPath: {
        root: 'dist',
      },
      // 热更新容器使用 Go embed 托管构建产物：前端 watch 产物和后端重编译之间
      // 会有一个很短的时间差。关闭文件名 hash 后，旧 HTML 仍能请求到新产物，
      // 避免开发联调时出现“HTML 旧、CSS/JS 文件已被清理”的无样式页面。
      filenameHash: isHotReloadBuild ? false : true,
      // Rely on Rsbuild default legalComments ("linked" → per-chunk *.LICENSE.txt) in all modes.
      // Do not set "none" in production: that strips minifier-preserved third-party notices and
      // extracted license files, which some distributions require for open-source compliance.
    },
    performance: {
      // 生产环境移除 console 输出，减少前端包体和运行时噪音。
      removeConsole: isProd ? ['log'] : false,
      // 禁用持久化构建缓存。该产物会被 Go embed 进后端二进制，HTML 中注入的
      // CSS/JS hash 必须与 dist 中真实文件完全一致；Rspack 持久化缓存曾在热更新
      // 容器中留下旧 CSS hash，导致页面加载不到样式。
      buildCache: false,
    },
    tools: {
      rspack: {
        plugins: [
          tanstackRouter({
            target: 'react',
            // Dev: avoid per-route async chunks (reduces white flash on navigation + faster HMR feedback).
            // Prod: keep route-based code splitting.
            autoCodeSplitting: isProd,
          }),
        ],
      },
    },
  }
})
