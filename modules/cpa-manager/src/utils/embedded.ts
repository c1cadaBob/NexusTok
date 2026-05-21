export const isNexusTokEmbedded = import.meta.env.VITE_NEXUSTOK_EMBEDDED === 'true';

export const NEXUSTOK_EMBEDDED_API_BASE = '/api/account-pool/management';

export const getNexusTokEmbeddedOrigin = (): string => {
  if (typeof window === 'undefined') return '';
  return window.location.origin;
};

export const getNexusTokUserId = (): string => {
  if (typeof window === 'undefined') return '';
  try {
    return window.localStorage.getItem('uid') || '';
  } catch {
    return '';
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
  const currentPath = `${window.location.pathname}${window.location.search}${window.location.hash}`;
  void resolveNexusTokLoginPath().then((loginPath) => {
    window.location.href = `${loginPath}?redirect=${encodeURIComponent(currentPath)}`;
  });
};
