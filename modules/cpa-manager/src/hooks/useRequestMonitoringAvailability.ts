import { useEffect, useMemo, useState } from 'react';
import {
  isUsageServiceId,
  normalizeUsageServiceBase,
  usageServiceApi,
} from '@/services/api/usageService';
import { useAuthStore, useUsageServiceStore } from '@/stores';
import { detectApiBaseFromLocation } from '@/utils/connection';
import { isNexusTokEmbedded } from '@/utils/embedded';

export type RequestMonitoringUnavailableReason =
  | 'checking'
  | 'service_not_configured'
  | 'service_unavailable'
  | 'monitoring_disabled';

export interface RequestMonitoringAvailability {
  checking: boolean;
  available: boolean;
  serviceBase: string;
  reason: RequestMonitoringUnavailableReason | '';
}

export function useRequestMonitoringAvailability(): RequestMonitoringAvailability {
  const apiBase = useAuthStore((state) => state.apiBase);
  const managementKey = useAuthStore((state) => state.managementKey);
  const usageServiceEnabled = useUsageServiceStore((state) => state.enabled);
  const usageServiceBase = useUsageServiceStore((state) => state.serviceBase);
  const usageServiceRevision = useUsageServiceStore((state) => state.revision);
  const [state, setState] = useState<RequestMonitoringAvailability>({
    checking: true,
    available: false,
    serviceBase: '',
    reason: 'checking',
  });

  const candidates = useMemo(() => {
    if (isNexusTokEmbedded) {
      const embeddedBase = normalizeUsageServiceBase(detectApiBaseFromLocation());
      return embeddedBase ? [embeddedBase] : [];
    }

    return Array.from(
      new Set(
        [
          usageServiceEnabled && usageServiceBase ? usageServiceBase : '',
          apiBase,
          detectApiBaseFromLocation(),
        ]
          .map((value) => normalizeUsageServiceBase(value || ''))
          .filter(Boolean)
      )
    );
  }, [apiBase, usageServiceBase, usageServiceEnabled]);

  useEffect(() => {
    let cancelled = false;

    const detect = async () => {
      if ((!isNexusTokEmbedded && !managementKey) || candidates.length === 0) {
        setState({
          checking: false,
          available: false,
          serviceBase: '',
          reason: 'service_not_configured',
        });
        return;
      }

      setState((current) => ({ ...current, checking: true, reason: 'checking' }));
      const hasConfiguredUsageService = Boolean(usageServiceEnabled && usageServiceBase);

      for (const candidate of candidates) {
        try {
          const info = await usageServiceApi.getInfo(candidate);
          if (!isUsageServiceId(info.service)) {
            continue;
          }
          if (isNexusTokEmbedded) {
            // NexusTok 嵌入模式下，Usage Service 只通过主项目同源代理暴露给浏览器。
            // 浏览器侧不会保存 CPAMC 独立部署时使用的 managementKey，也不应该重新校验
            // CPA 连接密钥是否存在；这些权限、内部密钥注入和上游可达性检查都由
            // NexusTok 后端代理完成。这里保持一个更贴近嵌入形态的不变量：
            // 只要同源 /usage-service/info 返回合法 Usage Service 身份，且服务声明
            // configured !== false，就认为请求监控入口可用。后续 /status 和
            // /v0/management/usage 若出现真实错误，会由数据加载 hook 展示具体错误。
            if (cancelled) return;
            const configured = info.configured !== false;
            setState({
              checking: false,
              available: configured,
              serviceBase: configured ? candidate : '',
              reason: configured ? '' : 'service_not_configured',
            });
            return;
          }
          const response = await usageServiceApi.getManagerConfig(candidate, managementKey);
          const collectorEnabled = response.config.collector?.enabled !== false;
          const hasCPAConnection = Boolean(
            response.config.cpaConnection?.cpaBaseUrl &&
              response.config.cpaConnection?.managementKey
          );
          if (cancelled) return;
          setState({
            checking: false,
            available: collectorEnabled && hasCPAConnection,
            serviceBase: candidate,
            reason: !collectorEnabled
              ? 'monitoring_disabled'
              : hasCPAConnection
                ? ''
                : 'service_not_configured',
          });
          return;
        } catch {
          // 普通 CPA 面板或不可达的外部 Usage Service 会在循环结束后统一归类为不可用。
        }
      }

      if (cancelled) return;
      setState({
        checking: false,
        available: false,
        serviceBase: '',
        reason: hasConfiguredUsageService ? 'service_unavailable' : 'service_not_configured',
      });
    };

    void detect();

    return () => {
      cancelled = true;
    };
  }, [candidates, managementKey, usageServiceBase, usageServiceEnabled, usageServiceRevision]);

  return state;
}
