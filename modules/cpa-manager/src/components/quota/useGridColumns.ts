import { useState, useEffect, useCallback } from 'react';

/**
 * 根据网格容器宽度和卡片最小宽度计算当前列数。
 *
 * 这个 hook 主要服务配额页的分页体验：页面在宽屏下可以展示多列卡片，
 * 在窄屏下会收敛为单列。分页大小需要跟随实际列数调整，才能保持
 * “每页约三行”的视觉节奏，避免窗口变宽后单页仍然只显示少量卡片，
 * 或窗口变窄后一次塞入过多卡片导致滚动体验变差。
 *
 * 返回值包含当前列数和 ref callback；调用方把 ref 挂到 grid 容器上即可。
 */
export function useGridColumns(
    itemMinWidth: number,
    gap: number = 16
): [number, (node: HTMLDivElement | null) => void] {
    const [columns, setColumns] = useState(1);
    const [element, setElement] = useState<HTMLDivElement | null>(null);

    const refCallback = useCallback((node: HTMLDivElement | null) => {
        setElement(node);
    }, []);

    useEffect(() => {
        if (!element) return;

        const updateColumns = () => {
            const containerWidth = element.clientWidth;
            const effectiveItemWidth = itemMinWidth + gap;
            const count = Math.floor((containerWidth + gap) / effectiveItemWidth);
            setColumns(Math.max(1, count));
        };

        updateColumns();

        const observer = new ResizeObserver(() => {
            updateColumns();
        });

        observer.observe(element);

        return () => observer.disconnect();
    }, [element, itemMinWidth, gap]);

    return [columns, refCallback];
}
