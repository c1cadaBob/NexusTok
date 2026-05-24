/*
Copyright (C) 2025 c1cada

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@c1cada.dev
*/

import React, { useCallback, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { THEME_WOOL_PAPER, useActualTheme, useTheme } from '../../context/Theme';

const ACCOUNT_POOL_MANAGER_URL = '/account-pool/manager/?embeddedFrame=true';
const ACCOUNT_POOL_PREFERENCES_EVENT = 'nexustok:account-pool-preferences';
const ACCOUNT_POOL_READY_EVENT = 'nexustok:account-pool-ready';
const PREFERENCE_SYNC_RETRY_DELAYS = [0, 100, 300, 700, 1500, 3000, 5000];

const AccountPool = () => {
  const { i18n, t } = useTranslation();
  const iframeRef = useRef(null);
  const theme = useTheme();
  const actualTheme = useActualTheme();

  const syncPreferencesToFrame = useCallback(() => {
    const target = iframeRef.current?.contentWindow;
    if (!target) return;

    target.postMessage(
      {
        type: ACCOUNT_POOL_PREFERENCES_EVENT,
        language: i18n.language,
        lang: i18n.language,
        resolvedTheme: actualTheme,
        themeMode: actualTheme,
        themePreset: theme === THEME_WOOL_PAPER ? THEME_WOOL_PAPER : 'default',
      },
      window.location.origin,
    );
  }, [actualTheme, i18n.language, theme]);

  const schedulePreferenceSync = useCallback(() => {
    const timers = PREFERENCE_SYNC_RETRY_DELAYS.map((delay) =>
      window.setTimeout(syncPreferencesToFrame, delay),
    );

    return () => timers.forEach((timer) => window.clearTimeout(timer));
  }, [syncPreferencesToFrame]);

  useEffect(() => {
    return schedulePreferenceSync();
  }, [schedulePreferenceSync]);

  useEffect(() => {
    const handleLanguageChanged = () => schedulePreferenceSync();
    i18n.on('languageChanged', handleLanguageChanged);
    return () => i18n.off('languageChanged', handleLanguageChanged);
  }, [i18n, schedulePreferenceSync]);

  useEffect(() => {
    const handleFrameMessage = (event) => {
      if (event.origin !== window.location.origin) return;
      if (event.source !== iframeRef.current?.contentWindow) return;
      if (event.data?.type !== ACCOUNT_POOL_READY_EVENT) return;
      schedulePreferenceSync();
    };

    window.addEventListener('message', handleFrameMessage);
    return () => window.removeEventListener('message', handleFrameMessage);
  }, [schedulePreferenceSync]);

  return (
    <div
      style={{
        height: 'calc(100vh - 88px)',
        marginTop: 40,
        minHeight: 0,
        overflow: 'hidden',
        width: '100%',
      }}
    >
      <iframe
        ref={iframeRef}
        title={t('账号池管理')}
        src={ACCOUNT_POOL_MANAGER_URL}
        allow='clipboard-read; clipboard-write'
        onLoad={schedulePreferenceSync}
        style={{
          width: '100%',
          height: '100%',
          border: 0,
          display: 'block',
          background: 'var(--semi-color-bg-0)',
        }}
      />
    </div>
  );
};

export default AccountPool;
