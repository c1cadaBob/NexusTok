import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { viteSingleFile } from 'vite-plugin-singlefile';
import path from 'path';
import { execSync } from 'child_process';
import fs from 'fs';

const cpaManagerMode = (process.env.VITE_CPA_MANAGER_MODE || '').trim().toLowerCase();
const embeddedFlag = (process.env.VITE_NEXUSTOK_EMBEDDED || '').trim().toLowerCase();
const isNexusTokEmbeddedBuild = embeddedFlag !== 'false' && cpaManagerMode !== 'standalone';

// 获取版本号，优先级依次为环境变量、git tag、package.json。
function getVersion(): string {
  // 1. 环境变量，通常由 GitHub Actions 或发布脚本注入。
  if (process.env.VERSION) {
    return process.env.VERSION;
  }

  // 2. 尝试读取 git tag，支持在 release 构建中自动带出版本。
  try {
    const gitTag = execSync('git describe --tags --exact-match 2>/dev/null || git describe --tags 2>/dev/null || echo ""', { encoding: 'utf8' }).trim();
    if (gitTag) {
      return gitTag;
    }
  } catch {
    // 当前环境可能没有 git，或仓库没有 tag，此时继续 fallback。
  }

  // 3. 最后回退到 package.json 版本号，避免无 git 环境下版本为空。
  try {
    const pkg = JSON.parse(fs.readFileSync(path.resolve(__dirname, 'package.json'), 'utf8'));
    if (pkg.version && pkg.version !== '0.0.0') {
      return pkg.version;
    }
  } catch {
    // package.json 不可读时使用 dev，保证构建不中断。
  }

  return 'dev';
}

// https://vitejs.dev/config/
export default defineConfig({
  base: isNexusTokEmbeddedBuild ? '/account-pool/manager/' : undefined,
  plugins: [
    react(),
    viteSingleFile({
      removeViteModuleLoader: true
    })
  ],
  define: {
    __APP_VERSION__: JSON.stringify(getVersion())
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src')
    }
  },
  css: {
    modules: {
      localsConvention: 'camelCase',
      generateScopedName: '[name]__[local]___[hash:base64:5]'
    },
    preprocessorOptions: {
      scss: {
        additionalData: `@use "@/styles/variables.scss" as *;`
      }
    }
  },
  build: {
    target: 'es2020',
    outDir: 'dist',
    assetsInlineLimit: 100000000,
    chunkSizeWarningLimit: 100000000,
    cssCodeSplit: false,
    rolldownOptions: {
      output: {
        codeSplitting: false
      }
    }
  }
});
