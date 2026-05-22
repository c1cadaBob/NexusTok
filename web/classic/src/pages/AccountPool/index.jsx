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

import React from 'react';
import { useTranslation } from 'react-i18next';

const ACCOUNT_POOL_MANAGER_URL = '/account-pool/manager/?embeddedFrame=true';

const AccountPool = () => {
  const { t } = useTranslation();

  return (
    <div
      className='mt-[60px]'
      style={{
        height: 'calc(100vh - 172px)',
        minHeight: 560,
        width: '100%',
      }}
    >
      <iframe
        title={t('账号池管理')}
        src={ACCOUNT_POOL_MANAGER_URL}
        allow='clipboard-read; clipboard-write'
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
