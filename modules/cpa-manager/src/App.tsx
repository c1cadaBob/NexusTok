import { useEffect } from 'react';
import { Navigate, Outlet, RouterProvider, createHashRouter } from 'react-router-dom';
import { LoginPage } from '@/pages/LoginPage';
import { NotificationContainer } from '@/components/common/NotificationContainer';
import { ConfirmationModal } from '@/components/common/ConfirmationModal';
import { MainLayout } from '@/components/layout/MainLayout';
import { ProtectedRoute } from '@/router/ProtectedRoute';
import { useLanguageStore, useThemeStore } from '@/stores';
import { isNexusTokEmbedded } from '@/utils/embedded';
import { isSupportedLanguage } from '@/utils/language';
import type { Language, Theme } from '@/types';

type NexusTokPreferencesMessage = {
  type?: string;
  language?: string;
  lang?: string;
  resolvedTheme?: string;
  themeMode?: string;
  themePreset?: string;
};

const NEXUSTOK_PREFERENCES_EVENT = 'nexustok:account-pool-preferences';
const NEXUSTOK_READY_EVENT = 'nexustok:account-pool-ready';

const normalizeNexusTokLanguage = (language: string | undefined): Language | null => {
  if (!language) return null;

  const normalized = language.trim().replace(/_/g, '-');
  const lower = normalized.toLowerCase();
  const candidate =
    lower === 'zh' || lower.startsWith('zh-cn') || lower.startsWith('zh-hans')
      ? 'zh-CN'
      : lower.startsWith('zh-tw') || lower.startsWith('zh-hk') || lower.startsWith('zh-hant')
        ? 'zh-TW'
        : lower.startsWith('ru')
          ? 'ru'
          : lower.startsWith('en')
            ? 'en'
            : null;

  return candidate && isSupportedLanguage(candidate) ? candidate : null;
};

const normalizeNexusTokTheme = (theme: string | undefined): Theme | null => {
  if (theme === 'dark') return 'dark';
  if (theme === 'light') return 'light';
  return null;
};

const applyNexusTokThemePreset = (preset: string | undefined) => {
  if (typeof document === 'undefined') return;

  if (!preset || preset === 'default') {
    delete document.documentElement.dataset.nexusTokThemePreset;
    return;
  }

  document.documentElement.dataset.nexusTokThemePreset = preset;
};

function RootShell() {
  return (
    <>
      <NotificationContainer />
      <ConfirmationModal />
      <Outlet />
    </>
  );
}

const router = createHashRouter([
  {
    element: <RootShell />,
    children: [
      { path: '/login', element: isNexusTokEmbedded ? <Navigate to="/" replace /> : <LoginPage /> },
      {
        path: '/*',
        element: (
          <ProtectedRoute>
            <MainLayout />
          </ProtectedRoute>
        ),
      },
    ],
  },
]);

function App() {
  const initializeTheme = useThemeStore((state) => state.initializeTheme);
  const setTheme = useThemeStore((state) => state.setTheme);
  const language = useLanguageStore((state) => state.language);
  const setLanguage = useLanguageStore((state) => state.setLanguage);

  useEffect(() => {
    const cleanupTheme = initializeTheme();
    return cleanupTheme;
  }, [initializeTheme]);

  useEffect(() => {
    setLanguage(language);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []); // 仅用于首屏同步 i18n 语言

  useEffect(() => {
    document.documentElement.lang = language;
  }, [language]);

  useEffect(() => {
    if (!isNexusTokEmbedded) return;

    const handleNexusTokPreferences = (event: MessageEvent<NexusTokPreferencesMessage>) => {
      if (event.origin !== window.location.origin) return;
      const payload = event.data;
      if (!payload || typeof payload !== 'object') return;

      const isModernMessage = payload.type === NEXUSTOK_PREFERENCES_EVENT;
      const hasLegacyPayload = 'themeMode' in payload || 'lang' in payload;
      if (!isModernMessage && !hasLegacyPayload) return;

      const nextLanguage = normalizeNexusTokLanguage(payload.language ?? payload.lang);
      if (nextLanguage) {
        setLanguage(nextLanguage);
      }

      const nextTheme = normalizeNexusTokTheme(payload.resolvedTheme ?? payload.themeMode);
      if (nextTheme) {
        setTheme(nextTheme);
      }

      if (isModernMessage || 'themePreset' in payload) {
        applyNexusTokThemePreset(payload.themePreset);
      }
    };

    window.addEventListener('message', handleNexusTokPreferences);
    return () => window.removeEventListener('message', handleNexusTokPreferences);
  }, [setLanguage, setTheme]);

  useEffect(() => {
    if (!isNexusTokEmbedded) return;

    window.parent.postMessage({ type: NEXUSTOK_READY_EVENT }, window.location.origin);
  }, []);

  return <RouterProvider router={router} />;
}

export default App;
