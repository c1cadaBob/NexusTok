const cpaManagerMode = (import.meta.env.VITE_CPA_MANAGER_MODE || '').trim().toLowerCase();
const embeddedFlag = (import.meta.env.VITE_NEXUSTOK_EMBEDDED || '').trim().toLowerCase();

// NexusTok 仓库内的 CPAMC 默认作为账号池管理模块嵌入主项目运行。
// 只有显式设置 VITE_NEXUSTOK_EMBEDDED=false 或 VITE_CPA_MANAGER_MODE=standalone 时，
// 才恢复 CPAMC 原本的独立登录模式；这样直接执行 npm run build 也不会生成需要重复登录的页面。
export const isNexusTokEmbedded = embeddedFlag !== 'false' && cpaManagerMode !== 'standalone';

export const NEXUSTOK_EMBEDDED_API_BASE = '/api/account-pool/management';

export const isNexusTokEmbeddedFrame = (): boolean => {
  if (typeof window === 'undefined') return false;

  try {
    if (window.self !== window.top) return true;
  } catch {
    return true;
  }

  return new URLSearchParams(window.location.search).get('embeddedFrame') === 'true';
};

export const getNexusTokEmbeddedOrigin = (): string => {
  if (typeof window === 'undefined') return '';
  return window.location.origin;
};

export const getNexusTokUserId = (): string => {
  if (typeof window === 'undefined') return '';
  try {
    const uid = window.localStorage.getItem('uid');
    if (uid) return uid;

    const userText = window.localStorage.getItem('user');
    if (!userText) return '';

    const user = JSON.parse(userText) as { id?: unknown };
    return user.id === undefined || user.id === null ? '' : String(user.id);
  } catch {
    return '';
  }
};

export const clearNexusTokEmbeddedAuthStorage = (): void => {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.removeItem('uid');
    window.localStorage.removeItem('user');
  } catch {
    // 忽略浏览器隐私模式或存储不可用导致的清理失败。
  }
};

const resolveNexusTokLoginPath = async (): Promise<string> => {
  try {
    const response = await fetch('/api/status', { credentials: 'include' });
    const payload = await response.json();
    return payload?.data?.theme === 'classic' ? '/login' : '/sign-in';
  } catch {
    return '/sign-in';
  }
};

export const redirectToNexusTokLogin = (): void => {
  if (typeof window === 'undefined') return;
  clearNexusTokEmbeddedAuthStorage();
  const currentPath = `${window.location.pathname}${window.location.search}${window.location.hash}`;
  void resolveNexusTokLoginPath().then((loginPath) => {
    window.location.href = `${loginPath}?redirect=${encodeURIComponent(currentPath)}`;
  });
};

const resolveNexusTokConsolePath = async (): Promise<string> => {
  try {
    const response = await fetch('/api/status', { credentials: 'include' });
    const payload = await response.json();
    return payload?.data?.theme === 'classic' ? '/console' : '/dashboard';
  } catch {
    return '/console';
  }
};

export const returnToNexusTokConsole = (): void => {
  if (typeof window === 'undefined') return;
  void resolveNexusTokConsolePath().then((consolePath) => {
    window.location.href = consolePath;
  });
};
