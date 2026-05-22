export const isNexusTokEmbedded = import.meta.env.VITE_NEXUSTOK_EMBEDDED === 'true';

export const NEXUSTOK_EMBEDDED_API_BASE = '/api/account-pool/management';

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
