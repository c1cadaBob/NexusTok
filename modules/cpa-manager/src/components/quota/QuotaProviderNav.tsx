import {
  CSSProperties,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from 'react';
import { createPortal } from 'react-dom';
import iconAntigravity from '@/assets/icons/antigravity.svg';
import iconClaude from '@/assets/icons/claude.svg';
import iconCodex from '@/assets/icons/codex.svg';
import iconGemini from '@/assets/icons/gemini.svg';
import iconKimiDark from '@/assets/icons/kimi-dark.svg';
import iconKimiLight from '@/assets/icons/kimi-light.svg';
import { useThemeStore } from '@/stores';
import type { QuotaType } from './quotaConfigs';
import styles from '@/pages/QuotaPage.module.scss';

interface QuotaProviderNavItem {
  id: QuotaType;
  label: string;
  getIcon: (theme: string) => string;
}

const QUOTA_PROVIDER_NAV_ITEMS: QuotaProviderNavItem[] = [
  { id: 'codex', label: 'Codex', getIcon: () => iconCodex },
  { id: 'claude', label: 'Claude', getIcon: () => iconClaude },
  { id: 'antigravity', label: 'Antigravity', getIcon: () => iconAntigravity },
  { id: 'gemini-cli', label: 'Gemini CLI', getIcon: () => iconGemini },
  { id: 'kimi', label: 'Kimi', getIcon: (theme) => (theme === 'dark' ? iconKimiDark : iconKimiLight) },
];

const QUOTA_NAV_SCROLL_OFFSET = 24;

type ScrollContainer = HTMLElement | (Window & typeof globalThis);

export function QuotaProviderNav() {
  const resolvedTheme = useThemeStore((state) => state.resolvedTheme);
  const [activeProvider, setActiveProvider] = useState<QuotaType | null>(null);
  const contentScrollerRef = useRef<HTMLElement | null>(null);
  const navContainerRef = useRef<HTMLDivElement | null>(null);
  const itemRefs = useRef<Record<QuotaType, HTMLButtonElement | null>>({
    antigravity: null,
    claude: null,
    codex: null,
    'gemini-cli': null,
    kimi: null,
  });
  const [indicatorRect, setIndicatorRect] = useState<{
    x: number;
    y: number;
    width: number;
    height: number;
  } | null>(null);
  const [indicatorTransitionsEnabled, setIndicatorTransitionsEnabled] = useState(false);
  const indicatorHasEnabledTransitionsRef = useRef(false);

  const getHeaderHeight = useCallback(() => {
    const header = document.querySelector('.main-header') as HTMLElement | null;
    if (header) return header.getBoundingClientRect().height;

    const raw = getComputedStyle(document.documentElement).getPropertyValue('--header-height');
    const value = Number.parseFloat(raw);
    return Number.isFinite(value) ? value : 0;
  }, []);

  const getContentScroller = useCallback(() => {
    if (contentScrollerRef.current && document.contains(contentScrollerRef.current)) {
      return contentScrollerRef.current;
    }

    const container = document.querySelector('.content') as HTMLElement | null;
    contentScrollerRef.current = container;
    return container;
  }, []);

  const getScrollContainer = useCallback((): ScrollContainer => {
    // 桌面端内容区由 `.content` 独立滚动；移动端布局切换后使用 document 滚动。
    const isMobile = window.matchMedia('(max-width: 768px)').matches;
    if (isMobile) return window;
    return getContentScroller() ?? window;
  }, [getContentScroller]);

  const updateIndicator = useCallback((providerId: QuotaType | null) => {
    if (!providerId) {
      setIndicatorRect(null);
      return;
    }

    const itemEl = itemRefs.current[providerId];
    if (!itemEl) return;

    setIndicatorRect({
      x: itemEl.offsetLeft,
      y: itemEl.offsetTop,
      width: itemEl.offsetWidth,
      height: itemEl.offsetHeight,
    });

    // 首次定位时禁用动画，避免从左上角闪动；定位完成后再启用后续滑动过渡。
    if (!indicatorHasEnabledTransitionsRef.current) {
      indicatorHasEnabledTransitionsRef.current = true;
      requestAnimationFrame(() => setIndicatorTransitionsEnabled(true));
    }
  }, []);

  const handleScroll = useCallback(() => {
    const container = getScrollContainer();
    if (!container) return;

    const isElementScroller = container instanceof HTMLElement;
    const headerHeight = isElementScroller ? 0 : getHeaderHeight();
    const containerTop = isElementScroller ? container.getBoundingClientRect().top : 0;
    const activationLine = containerTop + headerHeight + QUOTA_NAV_SCROLL_OFFSET + 1;
    let currentActive: QuotaType | null = null;

    for (const provider of QUOTA_PROVIDER_NAV_ITEMS) {
      const element = document.getElementById(`quota-provider-${provider.id}`);
      if (!element) continue;

      const rect = element.getBoundingClientRect();
      if (rect.top <= activationLine) {
        currentActive = provider.id;
        continue;
      }

      if (currentActive) break;
    }

    if (!currentActive) {
      const firstVisible = QUOTA_PROVIDER_NAV_ITEMS.find((provider) =>
        document.getElementById(`quota-provider-${provider.id}`)
      );
      currentActive = firstVisible?.id ?? null;
    }

    setActiveProvider(currentActive);
  }, [getHeaderHeight, getScrollContainer]);

  useEffect(() => {
    const contentScroller = getContentScroller();

    // 桌面端监听 `.content`，移动端监听 window；同时监听 resize 以处理侧栏折叠和窗口变化。
    window.addEventListener('scroll', handleScroll, { passive: true });
    contentScroller?.addEventListener('scroll', handleScroll, { passive: true });
    window.addEventListener('resize', handleScroll);
    const raf = requestAnimationFrame(handleScroll);

    return () => {
      cancelAnimationFrame(raf);
      window.removeEventListener('scroll', handleScroll);
      window.removeEventListener('resize', handleScroll);
      contentScroller?.removeEventListener('scroll', handleScroll);
    };
  }, [getContentScroller, handleScroll]);

  useLayoutEffect(() => {
    const raf = requestAnimationFrame(() => updateIndicator(activeProvider));
    return () => cancelAnimationFrame(raf);
  }, [activeProvider, updateIndicator]);

  useLayoutEffect(() => {
    const el = navContainerRef.current;
    if (!el) return;

    const updateHeight = () => {
      const height = el.getBoundingClientRect().height;
      document.documentElement.style.setProperty('--quota-provider-nav-height', `${height}px`);
    };

    updateHeight();
    window.addEventListener('resize', updateHeight);

    const resizeObserver =
      typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(updateHeight);
    resizeObserver?.observe(el);

    return () => {
      resizeObserver?.disconnect();
      window.removeEventListener('resize', updateHeight);
      document.documentElement.style.removeProperty('--quota-provider-nav-height');
    };
  }, []);

  useEffect(() => {
    const handleResize = () => updateIndicator(activeProvider);
    window.addEventListener('resize', handleResize);
    return () => {
      window.removeEventListener('resize', handleResize);
    };
  }, [activeProvider, updateIndicator]);

  const scrollToProvider = (providerId: QuotaType) => {
    const container = getScrollContainer();
    const element = document.getElementById(`quota-provider-${providerId}`);
    if (!element || !container) return;

    setActiveProvider(providerId);
    updateIndicator(providerId);

    if (!(container instanceof HTMLElement)) {
      const headerHeight = getHeaderHeight();
      const elementTop = element.getBoundingClientRect().top + window.scrollY;
      const target = Math.max(0, elementTop - headerHeight - QUOTA_NAV_SCROLL_OFFSET);
      window.scrollTo({ top: target, behavior: 'smooth' });
      return;
    }

    const containerRect = container.getBoundingClientRect();
    const elementRect = element.getBoundingClientRect();
    const scrollTop =
      container.scrollTop + (elementRect.top - containerRect.top) - QUOTA_NAV_SCROLL_OFFSET;

    container.scrollTo({ top: scrollTop, behavior: 'smooth' });
  };

  const navContent = (
    <div className={styles.providerNavContainer} ref={navContainerRef}>
      <div className={styles.providerNavList}>
        <div
          className={[
            styles.providerNavIndicator,
            indicatorRect ? styles.providerNavIndicatorVisible : '',
            indicatorTransitionsEnabled ? '' : styles.providerNavIndicatorNoTransition,
          ]
            .filter(Boolean)
            .join(' ')}
          style={
            (indicatorRect
              ? ({
                  transform: `translate3d(${indicatorRect.x}px, ${indicatorRect.y}px, 0)`,
                  width: indicatorRect.width,
                  height: indicatorRect.height,
                } satisfies CSSProperties)
              : undefined) as CSSProperties | undefined
          }
        />
        {QUOTA_PROVIDER_NAV_ITEMS.map((provider) => {
          const isActive = activeProvider === provider.id;
          return (
            <button
              key={provider.id}
              className={`${styles.providerNavItem} ${isActive ? styles.providerNavItemActive : ''}`}
              ref={(node) => {
                itemRefs.current[provider.id] = node;
              }}
              onClick={() => scrollToProvider(provider.id)}
              title={provider.label}
              type="button"
              aria-label={provider.label}
              aria-pressed={isActive}
            >
              <img
                src={provider.getIcon(resolvedTheme)}
                alt={provider.label}
                className={styles.providerNavIcon}
              />
            </button>
          );
        })}
      </div>
    </div>
  );

  if (typeof document === 'undefined') return null;

  return createPortal(navContent, document.body);
}
