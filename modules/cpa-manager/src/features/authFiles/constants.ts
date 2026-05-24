import type { TFunction } from 'i18next';
import iconAntigravity from '@/assets/icons/antigravity.svg';
import iconClaude from '@/assets/icons/claude.svg';
import iconCodex from '@/assets/icons/codex.svg';
import iconAmp from '@/assets/icons/amp.svg';
import iconGemini from '@/assets/icons/gemini.svg';
import iconGrok from '@/assets/icons/grok.svg';
import iconGrokDark from '@/assets/icons/grok-dark.svg';
import iconIflow from '@/assets/icons/iflow.svg';
import iconKiro from '@/assets/icons/kiro.svg';
import iconKimiDark from '@/assets/icons/kimi-dark.svg';
import iconKimiLight from '@/assets/icons/kimi-light.svg';
import iconOpenaiDark from '@/assets/icons/openai-dark.svg';
import iconOpenaiLight from '@/assets/icons/openai-light.svg';
import iconQwen from '@/assets/icons/qwen.svg';
import iconVertex from '@/assets/icons/vertex.svg';
import type { AuthFileItem } from '@/types';
import { parseTimestamp } from '@/utils/timestamp';

export type ThemeColors = { bg: string; text: string; border?: string };
export type TypeColorSet = { light: ThemeColors; dark?: ThemeColors };
export type ResolvedTheme = 'light' | 'dark';
export type AuthFileModelItem = {
  id: string;
  display_name?: string;
  type?: string;
  owned_by?: string;
};
export type AuthFileIconAsset = string | { light: string; dark: string };

export type QuotaProviderType = 'antigravity' | 'claude' | 'codex' | 'gemini-cli' | 'kimi';

export const QUOTA_PROVIDER_TYPES = new Set<QuotaProviderType>([
  'antigravity',
  'claude',
  'codex',
  'gemini-cli',
  'kimi',
]);

export const MIN_CARD_PAGE_SIZE = 3;
export const MAX_CARD_PAGE_SIZE = 30;
export const AUTH_FILE_REFRESH_WARNING_MS = 24 * 60 * 60 * 1000;

export const INTEGER_STRING_PATTERN = /^[+-]?\d+$/;
export const TRUTHY_TEXT_VALUES = new Set(['true', '1', 'yes', 'y', 'on']);
export const FALSY_TEXT_VALUES = new Set(['false', '0', 'no', 'n', 'off']);

// 标签类型颜色配置：基于各提供商 Logo 品牌色调配，确保列表中不同账号类型有稳定且容易区分的视觉提示。
export const TYPE_COLORS: Record<string, TypeColorSet> = {
  // Qwen logo 使用紫罗兰渐变 #6336E7 到 #6F69F7，因此标签采用同一色相的浅底深字。
  qwen: {
    light: { bg: '#ede5fd', text: '#5530c7' },
    dark: { bg: '#36208a', text: '#b5a3f0' },
  },
  // Kimi logo 主色为亮蓝 #027AFF，标签保留蓝色识别度但降低背景饱和度。
  kimi: {
    light: { bg: '#dce8ff', text: '#0560cf' },
    dark: { bg: '#003880', text: '#70b5ff' },
  },
  // Gemini logo 使用偏柔和的多色蓝，这里选取 #3186FF 附近的蓝色作为主识别色。
  gemini: {
    light: { bg: '#e3f2fd', text: '#1565c0' },
    dark: { bg: '#0d47a1', text: '#64b5f6' },
  },
  // Gemini-CLI 与 Gemini 共用图标，但在认证文件里代表官方账号凭证，使用更深海军蓝做区分。
  'gemini-cli': {
    light: { bg: '#e0e8ff', text: '#1e4fa3' },
    dark: { bg: '#1c3f73', text: '#a8c7ff' },
  },
  // AI Studio 也复用 Gemini 图标，但执行路径不同，使用中性灰避免误认为 Gemini API Key。
  aistudio: {
    light: { bg: '#f0f2f5', text: '#2f343c' },
    dark: { bg: '#373c42', text: '#cfd3db' },
  },
  // Claude logo 主色接近陶土橙 #D97757，标签使用暖色以便和蓝色系供应商拉开距离。
  claude: {
    light: { bg: '#fbece4', text: '#c05621' },
    dark: { bg: '#5e2c14', text: '#e8a882' },
  },
  // Codex logo 使用靛蓝渐变 #B1A7FF 到 #3941FF，因此标签采用更稳定的靛蓝文字。
  codex: {
    light: { bg: '#eae7ff', text: '#3538d4' },
    dark: { bg: '#262395', text: '#b5b0ff' },
  },
  // Antigravity logo 同时包含蓝色和青绿色，这里优先使用青色，避免与 Gemini/Codex 过度相似。
  antigravity: {
    light: { bg: '#e0f7fa', text: '#006064' },
    dark: { bg: '#004d40', text: '#80deea' },
  },
  // xAI / Grok：使用石墨色系，避免和蓝色、紫色供应商混在一起。
  xai: {
    light: { bg: '#f3f4f6', text: '#111827', border: '1px solid #d1d5db' },
    dark: { bg: '#111827', text: '#f9fafb', border: '1px solid #374151' },
  },
  // iFlow logo 为品红紫渐变 #5C5CFF 到 #AE5CFF，这里偏向品红以区别于 Qwen 紫罗兰。
  iflow: {
    light: { bg: '#f5e3fc', text: '#9025c8' },
    dark: { bg: '#521490', text: '#d49cf5' },
  },
  // Vertex logo 使用 Google 蓝 #4285F4，标签保留官方视觉线索。
  vertex: {
    light: { bg: '#e4edfd', text: '#2b5fbc' },
    dark: { bg: '#1a3d80', text: '#89b3f7' },
  },
  // Kiro 先作为账号池官方账号预留类型展示，使用青蓝和紫色之间的折中色，
  // 与底部配额导航中的预留图标保持一致，避免误认为已经接入的 Vertex 或 Gemini。
  kiro: {
    light: { bg: '#e0f2fe', text: '#075985', border: '1px solid #bae6fd' },
    dark: { bg: '#0c4a6e', text: '#bae6fd', border: '1px solid #0369a1' },
  },
  // OpenAI 兼容上游：使用中性黑白标签，和实际 OpenAI 图标保持一致。
  openai: {
    light: { bg: '#f2f3f5', text: '#202123', border: '1px solid #d4d4d8' },
    dark: { bg: '#202123', text: '#f8fafc', border: '1px solid #3f3f46' },
  },
  // Ampcode：使用偏绿色标签，和账号/OAuth 类型区分开，表示它更接近上游路由配置。
  ampcode: {
    light: { bg: '#e8f7ef', text: '#137447' },
    dark: { bg: '#0f3f2a', text: '#8ee6b5' },
  },
  empty: {
    light: { bg: '#f5f5f5', text: '#616161' },
    dark: { bg: '#424242', text: '#bdbdbd' },
  },
  unknown: {
    light: { bg: '#f0f0f0', text: '#666666', border: '1px dashed #999999' },
    dark: { bg: '#3a3a3a', text: '#aaaaaa', border: '1px dashed #666666' },
  },
};

export const AUTH_FILE_ICONS: Record<string, AuthFileIconAsset> = {
  antigravity: iconAntigravity,
  aistudio: iconGemini,
  claude: iconClaude,
  codex: iconCodex,
  gemini: iconGemini,
  'gemini-cli': iconGemini,
  xai: { light: iconGrok, dark: iconGrokDark },
  iflow: iconIflow,
  kimi: { light: iconKimiLight, dark: iconKimiDark },
  openai: { light: iconOpenaiLight, dark: iconOpenaiDark },
  qwen: iconQwen,
  vertex: iconVertex,
  kiro: iconKiro,
  ampcode: iconAmp,
};

// CLIProxyAPI 的 /model-definitions/:channel 只为下列 channel 提供静态模型列表。
// 其他 provider（例如 Qwen、iFlow）仍然可以手动维护规则，但前端不能请求不存在的
// 模型定义接口，否则管理代理会返回 400 并在嵌入页面里产生误导性的错误提示。
const MODEL_DEFINITION_SUPPORTED_PROVIDERS = new Set([
  'claude',
  'gemini',
  'vertex',
  'gemini-cli',
  'aistudio',
  'codex',
  'kimi',
  'antigravity',
  'xai',
]);

// OAuth 模型别名只有在 CLIProxyAPI 执行链路会读取 alias channel 时才真正生效。
// API Key 型上游或尚未实现官方账号执行器的类型可以继续管理认证文件和禁用规则，
// 但不应在快捷入口中暗示其模型别名已经会被 relay 使用。
const OAUTH_MODEL_ALIAS_SUPPORTED_PROVIDERS = new Set([
  'gemini-cli',
  'vertex',
  'aistudio',
  'antigravity',
  'claude',
  'codex',
  'kimi',
]);

export const supportsModelDefinitions = (provider: string): boolean =>
  MODEL_DEFINITION_SUPPORTED_PROVIDERS.has(normalizeProviderKey(provider));

export const supportsOAuthModelAlias = (provider: string): boolean =>
  OAUTH_MODEL_ALIAS_SUPPORTED_PROVIDERS.has(normalizeProviderKey(provider));

export const clampCardPageSize = (value: number) =>
  Math.min(MAX_CARD_PAGE_SIZE, Math.max(MIN_CARD_PAGE_SIZE, Math.round(value)));

export const resolveQuotaErrorMessage = (
  t: TFunction,
  status: number | undefined,
  fallback: string
): string => {
  if (status === 404) return t('common.quota_update_required');
  if (status === 403) return t('common.quota_check_credential');
  return fallback;
};

export const normalizeProviderKey = (value: string) => {
  const key = value.trim().toLowerCase().replace(/_/g, '-');
  if (key === 'x-ai' || key === 'grok') return 'xai';
  return key;
};

export const getAuthFileStatusMessage = (file: AuthFileItem): string => {
  const raw = file['status_message'] ?? file.statusMessage;
  if (typeof raw === 'string') return raw.trim();
  if (raw == null) return '';
  return String(raw).trim();
};

export const hasAuthFileStatusMessage = (file: AuthFileItem): boolean =>
  getAuthFileStatusMessage(file).length > 0;

export const isHealthyAuthFile = (file: AuthFileItem): boolean =>
  file.disabled !== true && !hasAuthFileStatusMessage(file);

export const getTypeLabel = (t: TFunction, type: string): string => {
  const providerKey = normalizeProviderKey(type);
  const key = `auth_files.filter_${providerKey}`;
  const translated = t(key);
  if (translated !== key) return translated;
  if (providerKey === 'iflow') return 'iFlow';
  return type.charAt(0).toUpperCase() + type.slice(1);
};

export const getTypeColor = (type: string, resolvedTheme: ResolvedTheme): ThemeColors => {
  const set = TYPE_COLORS[normalizeProviderKey(type)] || TYPE_COLORS.unknown;
  return resolvedTheme === 'dark' && set.dark ? set.dark : set.light;
};

export const getAuthFileIcon = (type: string, resolvedTheme: ResolvedTheme): string | null => {
  const iconEntry = AUTH_FILE_ICONS[normalizeProviderKey(type)];
  if (!iconEntry) return null;
  return typeof iconEntry === 'string'
    ? iconEntry
    : resolvedTheme === 'dark'
      ? iconEntry.dark
      : iconEntry.light;
};

export const parsePriorityValue = (value: unknown): number | undefined => {
  if (typeof value === 'number') {
    return Number.isInteger(value) ? value : undefined;
  }

  if (typeof value !== 'string') return undefined;
  const trimmed = value.trim();
  if (!trimmed || !INTEGER_STRING_PATTERN.test(trimmed)) return undefined;
  const parsed = Number.parseInt(trimmed, 10);
  return Number.isSafeInteger(parsed) ? parsed : undefined;
};

export const normalizeExcludedModels = (value: unknown): string[] => {
  if (!Array.isArray(value)) return [];

  const seen = new Set<string>();
  const normalized: string[] = [];
  value.forEach((entry) => {
    const model = String(entry ?? '')
      .trim()
      .toLowerCase();
    if (!model || seen.has(model)) return;
    seen.add(model);
    normalized.push(model);
  });

  return normalized.sort((a, b) => a.localeCompare(b));
};

export const parseExcludedModelsText = (value: string): string[] =>
  normalizeExcludedModels(value.split(/[\n,]+/));

export const parseDisableCoolingValue = (value: unknown): boolean | undefined => {
  if (typeof value === 'boolean') return value;
  if (typeof value === 'number' && Number.isFinite(value)) return value !== 0;
  if (typeof value !== 'string') return undefined;

  const normalized = value.trim().toLowerCase();
  if (!normalized) return undefined;
  if (TRUTHY_TEXT_VALUES.has(normalized)) return true;
  if (FALSY_TEXT_VALUES.has(normalized)) return false;
  return undefined;
};

export const readCodexAuthFileWebsockets = (value: Record<string, unknown>): boolean =>
  parseDisableCoolingValue(value.websockets) ?? false;

export const applyCodexAuthFileWebsockets = (
  value: Record<string, unknown>,
  websockets: boolean
): Record<string, unknown> => {
  const next = { ...value };
  delete next.websocket;
  next.websockets = websockets;
  return next;
};

export function isRuntimeOnlyAuthFile(file: AuthFileItem): boolean {
  const raw = file['runtime_only'] ?? file.runtimeOnly;
  if (typeof raw === 'boolean') return raw;
  if (typeof raw === 'string') return raw.trim().toLowerCase() === 'true';
  return false;
}

export const formatModified = (item: AuthFileItem): string => {
  const raw = item['modtime'] ?? item.modified;
  if (!raw) return '-';
  const asNumber = Number(raw);
  const date =
    Number.isFinite(asNumber) && !Number.isNaN(asNumber)
      ? new Date(asNumber < 1e12 ? asNumber * 1000 : asNumber)
      : parseTimestamp(raw) ?? new Date(String(raw));
  return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString();
};

// 检查模型是否被 OAuth 排除
export const isModelExcluded = (
  modelId: string,
  providerType: string,
  excluded: Record<string, string[]>
): boolean => {
  const providerKey = normalizeProviderKey(providerType);
  const excludedModels = excluded[providerKey] || excluded[providerType] || [];
  return excludedModels.some((pattern) => {
    if (pattern.includes('*')) {
      // 支持通配符匹配：先转义正则特殊字符，再将 * 视为通配符
      const regexSafePattern = pattern
        .split('*')
        .map((segment) => segment.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'))
        .join('.*');
      const regex = new RegExp(`^${regexSafePattern}$`, 'i');
      return regex.test(modelId);
    }
    return pattern.toLowerCase() === modelId.toLowerCase();
  });
};
