import {
  ReactNode,
  SVGProps,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from 'react';
import { NavLink, useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import { PageTransition } from '@/components/common/PageTransition';
import { MainRoutes } from '@/router/MainRoutes';
import {
  IconCrosshair,
  IconSidebarAccountGroups,
  IconSidebarAuthFiles,
  IconSidebarConfig,
  IconSidebarDashboard,
  IconSidebarLogs,
  IconSidebarMonitor,
  IconSidebarOauth,
  IconSidebarQuota,
  IconSidebarSystem,
} from '@/components/ui/icons';
import { INLINE_LOGO_JPEG } from '@/assets/logoInline';
import {
  useAuthStore,
  useConfigStore,
} from '@/stores';
import {
  isNexusTokEmbedded,
  isNexusTokEmbeddedFrame,
  returnToNexusTokConsole,
} from '@/utils/embedded';

const sidebarIcons: Record<string, ReactNode> = {
  dashboard: <IconSidebarDashboard size={18} />,
  authFiles: <IconSidebarAuthFiles size={18} />,
  accountGroups: <IconSidebarAccountGroups size={18} />,
  oauth: <IconSidebarOauth size={18} />,
  quota: <IconSidebarQuota size={18} />,
  monitoring: <IconSidebarMonitor size={18} />,
  inspection: <IconCrosshair size={18} />,
  config: <IconSidebarConfig size={18} />,
  logs: <IconSidebarLogs size={18} />,
  system: <IconSidebarSystem size={18} />,
};

const headerIconProps: SVGProps<SVGSVGElement> = {
  width: 16,
  height: 16,
  viewBox: '0 0 24 24',
  fill: 'none',
  stroke: 'currentColor',
  strokeWidth: 2,
  strokeLinecap: 'round',
  strokeLinejoin: 'round',
  'aria-hidden': 'true',
  focusable: 'false',
};

const headerIcons = {
  menu: (
    <svg {...headerIconProps}>
      <path d="M4 7h16" />
      <path d="M4 12h16" />
      <path d="M4 17h16" />
    </svg>
  ),
  close: (
    <svg {...headerIconProps}>
      <path d="M18 6 6 18" />
      <path d="m6 6 12 12" />
    </svg>
  ),
  chevronLeft: (
    <svg {...headerIconProps}>
      <path d="m14 18-6-6 6-6" />
    </svg>
  ),
  chevronRight: (
    <svg {...headerIconProps}>
      <path d="m10 6 6 6-6 6" />
    </svg>
  ),
  logout: (
    <svg {...headerIconProps}>
      <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
      <path d="m16 17 5-5-5-5" />
      <path d="M21 12H9" />
    </svg>
  ),
  home: (
    <svg {...headerIconProps}>
      <path d="m3 11 9-8 9 8" />
      <path d="M5 10v10h14V10" />
      <path d="M9 20v-6h6v6" />
    </svg>
  ),
};

export function MainLayout() {
  const { t } = useTranslation();
  const location = useLocation();

  const logout = useAuthStore((state) => state.logout);

  const config = useConfigStore((state) => state.config);
  const fetchConfig = useConfigStore((state) => state.fetchConfig);

  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const contentRef = useRef<HTMLDivElement | null>(null);
  const headerRef = useRef<HTMLElement | null>(null);

  const fullBrandName = 'CLI Proxy API Management Center';
  const abbrBrandName = t('title.abbr');
  const isLogsPage = location.pathname.startsWith('/logs');
  const embeddedFrame = isNexusTokEmbeddedFrame();
  const showSidebarLabels = !sidebarCollapsed || sidebarOpen;

  // 将顶部悬浮控制区高度写入 CSS 变量，供移动端粘性元素和浮层避让。
  useLayoutEffect(() => {
    const updateHeaderHeight = () => {
      const height = headerRef.current?.offsetHeight;
      if (height) {
        document.documentElement.style.setProperty('--header-height', `${height}px`);
      }
    };

    updateHeaderHeight();

    const resizeObserver =
      typeof ResizeObserver !== 'undefined' && headerRef.current
        ? new ResizeObserver(updateHeaderHeight)
        : null;
    if (resizeObserver && headerRef.current) {
      resizeObserver.observe(headerRef.current);
    }

    window.addEventListener('resize', updateHeaderHeight);

    return () => {
      if (resizeObserver) {
        resizeObserver.disconnect();
      }
      window.removeEventListener('resize', updateHeaderHeight);
    };
  }, []);

  // 将主内容区的中心点写入 CSS 变量，供底部浮层（配置面板操作栏、提供商导航）对齐到内容区
  useLayoutEffect(() => {
    const updateContentCenter = () => {
      const el = contentRef.current;
      if (!el) return;
      const rect = el.getBoundingClientRect();
      const centerX = rect.left + rect.width / 2;
      document.documentElement.style.setProperty('--content-center-x', `${centerX}px`);
    };

    updateContentCenter();

    const resizeObserver =
      typeof ResizeObserver !== 'undefined' && contentRef.current
        ? new ResizeObserver(updateContentCenter)
        : null;

    if (resizeObserver && contentRef.current) {
      resizeObserver.observe(contentRef.current);
    }

    window.addEventListener('resize', updateContentCenter);

    return () => {
      if (resizeObserver) {
        resizeObserver.disconnect();
      }
      window.removeEventListener('resize', updateContentCenter);
      document.documentElement.style.removeProperty('--content-center-x');
    };
  }, []);

  useEffect(() => {
    fetchConfig().catch(() => {
      // 初始化配置失败时不阻断布局渲染；登录态或连接状态会在页面内给出提示。
    });
  }, [fetchConfig]);

  useEffect(() => {
    if (!embeddedFrame) return undefined;

    const root = document.documentElement;
    const previousProviderNavBottom = root.style.getPropertyValue('--quota-provider-nav-bottom');
    const previousProviderNavContentGap = root.style.getPropertyValue(
      '--quota-provider-nav-content-gap'
    );

    // 配额页的供应商快速导航通过 React Portal 挂载到 document.body。
    // Portal 节点不在 `.app-shell-embedded-frame` 的 DOM 继承链上，因此无法读取该容器
    // 内声明的 CSS 变量。这里把嵌入模式专用的底部定位变量同步到 documentElement，
    // 保证浮层在嵌入到 NexusTok 后也能贴近 iframe 底部，而独立运行模式仍保持原样。
    root.style.setProperty('--quota-provider-nav-bottom', '4px');
    root.style.setProperty('--quota-provider-nav-content-gap', '4px');

    return () => {
      if (previousProviderNavBottom) {
        root.style.setProperty('--quota-provider-nav-bottom', previousProviderNavBottom);
      } else {
        root.style.removeProperty('--quota-provider-nav-bottom');
      }

      if (previousProviderNavContentGap) {
        root.style.setProperty('--quota-provider-nav-content-gap', previousProviderNavContentGap);
      } else {
        root.style.removeProperty('--quota-provider-nav-content-gap');
      }
    };
  }, [embeddedFrame]);

  const navItems = [
    { path: '/', label: t('nav.dashboard'), icon: sidebarIcons.dashboard, end: true },
    { path: '/config', label: t('nav.config_management'), icon: sidebarIcons.config },
    { path: '/auth-files', label: t('nav.auth_files'), icon: sidebarIcons.authFiles },
    { path: '/account-groups', label: t('nav.account_groups'), icon: sidebarIcons.accountGroups },
    { path: '/quota', label: t('nav.quota_management'), icon: sidebarIcons.quota },
    {
      path: '/monitoring',
      label: t('nav.monitoring_center'),
      icon: sidebarIcons.monitoring,
      end: true,
    },
    {
      path: '/monitoring/inspection',
      label: t('nav.account_inspection'),
      icon: sidebarIcons.inspection,
    },
    ...(config?.loggingToFile
      ? [{ path: '/logs', label: t('nav.logs'), icon: sidebarIcons.logs }]
      : []),
    { path: '/system', label: t('nav.system_info'), icon: sidebarIcons.system },
  ];
  const navOrder = navItems.map((item) => item.path);
  const getRouteOrder = (pathname: string) => {
    const trimmedPath =
      pathname.length > 1 && pathname.endsWith('/') ? pathname.slice(0, -1) : pathname;
    const normalizedPath = trimmedPath === '/dashboard' ? '/' : trimmedPath;

    const authFilesIndex = navOrder.indexOf('/auth-files');
    if (authFilesIndex !== -1) {
      if (normalizedPath === '/auth-files') return authFilesIndex;
      if (normalizedPath.startsWith('/auth-files/')) {
        if (normalizedPath.startsWith('/auth-files/oauth-excluded')) return authFilesIndex + 0.1;
        if (normalizedPath.startsWith('/auth-files/oauth-model-alias')) return authFilesIndex + 0.2;
        return authFilesIndex + 0.05;
      }
    }

    const exactIndex = navOrder.indexOf(normalizedPath);
    if (exactIndex !== -1) return exactIndex;
    const nestedIndex = navOrder.findIndex(
      (path) => path !== '/' && normalizedPath.startsWith(`${path}/`)
    );
    return nestedIndex === -1 ? null : nestedIndex;
  };

  const getTransitionVariant = useCallback((fromPathname: string, toPathname: string) => {
    const normalize = (pathname: string) => {
      const trimmed =
        pathname.length > 1 && pathname.endsWith('/') ? pathname.slice(0, -1) : pathname;
      return trimmed === '/dashboard' ? '/' : trimmed;
    };

    const from = normalize(fromPathname);
    const to = normalize(toPathname);
    const isAuthFiles = (pathname: string) =>
      pathname === '/auth-files' || pathname.startsWith('/auth-files/');
    if (isAuthFiles(from) && isAuthFiles(to)) return 'ios';
    return 'vertical';
  }, []);

  const mobileSidebarToggleLabel = sidebarOpen
    ? t('sidebar.toggle_collapse', { defaultValue: 'Close navigation' })
    : t('sidebar.toggle_expand', { defaultValue: 'Open navigation' });

  return (
    <div
      className={`app-shell ${sidebarCollapsed ? 'sidebar-is-collapsed' : ''} ${
        embeddedFrame ? 'app-shell-embedded-frame' : ''
      }`}
    >
      <header className="main-header" ref={headerRef}>
        <div className="mobile-sidebar-actions">
          <Button
            className="mobile-menu-btn"
            variant="ghost"
            size="sm"
            onClick={() => setSidebarOpen((prev) => !prev)}
            title={mobileSidebarToggleLabel}
            aria-label={mobileSidebarToggleLabel}
          >
            {sidebarOpen ? headerIcons.close : headerIcons.menu}
          </Button>
        </div>

        <div className="header-actions floating-actions">
          {isNexusTokEmbedded && !embeddedFrame ? (
            <Button
              variant="ghost"
              size="sm"
              onClick={returnToNexusTokConsole}
              title={t('header.return_to_nexustok')}
            >
              {headerIcons.home}
            </Button>
          ) : null}
          {!isNexusTokEmbedded ? (
            <Button variant="ghost" size="sm" onClick={logout} title={t('header.logout')}>
              {headerIcons.logout}
            </Button>
          ) : null}
        </div>
      </header>

      <div className="main-body">
        <button
          type="button"
          className={`sidebar-backdrop ${sidebarOpen ? 'visible' : ''}`}
          onClick={() => setSidebarOpen(false)}
          aria-label={t('common.close')}
          aria-hidden={!sidebarOpen}
          tabIndex={sidebarOpen ? 0 : -1}
        />

        <aside
          className={`sidebar ${sidebarOpen ? 'open' : ''} ${sidebarCollapsed ? 'collapsed' : ''}`}
        >
          <div className="sidebar-brand" title={fullBrandName}>
            <div className="sidebar-brand-main">
              <img src={INLINE_LOGO_JPEG} alt="CPAMC logo" className="sidebar-brand-logo" />
              {showSidebarLabels && <span className="sidebar-brand-title">{abbrBrandName}</span>}
            </div>
            <button
              type="button"
              className="sidebar-collapse-inline"
              onClick={() => setSidebarCollapsed((prev) => !prev)}
              title={
                sidebarCollapsed
                  ? t('sidebar.expand', { defaultValue: '展开' })
                  : t('sidebar.collapse', { defaultValue: '收起' })
              }
              aria-label={
                sidebarCollapsed
                  ? t('sidebar.expand', { defaultValue: '展开' })
                  : t('sidebar.collapse', { defaultValue: '收起' })
              }
            >
              {sidebarCollapsed ? headerIcons.chevronRight : headerIcons.chevronLeft}
            </button>
          </div>

          <div className="nav-section">
            {navItems.map((item) => (
              <NavLink
                key={item.path}
                to={item.path}
                end={item.end}
                className={({ isActive }) => `nav-item ${isActive ? 'active' : ''}`}
                onClick={() => setSidebarOpen(false)}
                title={showSidebarLabels ? undefined : item.label}
              >
                <span className="nav-icon">{item.icon}</span>
                {showSidebarLabels && <span className="nav-label">{item.label}</span>}
              </NavLink>
            ))}
          </div>
        </aside>

        <div className={`content${isLogsPage ? ' content-logs' : ''}`} ref={contentRef}>
          <main className={`main-content${isLogsPage ? ' main-content-logs' : ''}`}>
            <PageTransition
              render={(location) => <MainRoutes location={location} />}
              getRouteOrder={getRouteOrder}
              getTransitionVariant={getTransitionVariant}
              scrollContainerRef={contentRef}
            />
          </main>
        </div>
      </div>
    </div>
  );
}
