import { Navigate, useRoutes, type Location } from 'react-router-dom';
import { DashboardPage } from '@/pages/DashboardPage';
import { AccountGroupsPage } from '@/pages/AccountGroupsPage';
import { AuthFilesPage } from '@/pages/AuthFilesPage';
import { AuthFilesOAuthExcludedEditPage } from '@/pages/AuthFilesOAuthExcludedEditPage';
import { AuthFilesOAuthModelAliasEditPage } from '@/pages/AuthFilesOAuthModelAliasEditPage';
import { QuotaPage } from '@/pages/QuotaPage';
import { MonitoringCenterPage } from '@/pages/MonitoringCenterPage';
import { CodexInspectionPage } from '@/pages/CodexInspectionPage';
import { ConfigPage } from '@/pages/ConfigPage';
import { LogsPage } from '@/pages/LogsPage';
import { SystemPage } from '@/pages/SystemPage';

const mainRoutes = [
  { path: '/', element: <DashboardPage /> },
  { path: '/dashboard', element: <DashboardPage /> },
  { path: '/settings', element: <Navigate to="/config" replace /> },
  { path: '/api-keys', element: <Navigate to="/config" replace /> },
  { path: '/ai-providers', element: <Navigate to="/auth-files" replace /> },
  { path: '/ai-providers/*', element: <Navigate to="/auth-files" replace /> },
  { path: '/auth-files', element: <AuthFilesPage /> },
  { path: '/account-groups', element: <AccountGroupsPage /> },
  { path: '/auth-files/oauth-excluded', element: <AuthFilesOAuthExcludedEditPage /> },
  { path: '/auth-files/oauth-model-alias', element: <AuthFilesOAuthModelAliasEditPage /> },
  { path: '/oauth', element: <Navigate to="/quota" replace /> },
  { path: '/quota', element: <QuotaPage /> },
  { path: '/monitoring', element: <MonitoringCenterPage /> },
  { path: '/monitoring/codex-inspection', element: <CodexInspectionPage /> },
  { path: '/config', element: <ConfigPage /> },
  { path: '/logs', element: <LogsPage /> },
  { path: '/system', element: <SystemPage /> },
  { path: '*', element: <Navigate to="/" replace /> },
];

export function MainRoutes({ location }: { location?: Location }) {
  return useRoutes(mainRoutes, location);
}
