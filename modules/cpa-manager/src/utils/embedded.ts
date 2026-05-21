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

export const redirectToNexusTokLogin = (): void => {
  if (typeof window === 'undefined') return;
  const currentPath = `${window.location.pathname}${window.location.search}${window.location.hash}`;
  window.location.href = `/login?redirect=${encodeURIComponent(currentPath)}`;
};
