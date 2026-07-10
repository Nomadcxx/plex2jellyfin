import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  addSettingsPath,
  api,
  getSettingsPaths,
  getSettingsSection,
  PathKind,
  putSettingsSection,
  removeSettingsPath,
  SettingsSection,
  testSettingsConnection,
} from '@/lib/api/client';
import { components } from '@/types/api';

export type RuntimeAISettings = components['schemas']['AISettings'] & {
  endpoint?: string;
  primaryModel?: string;
  fallbackModel?: string;
};

type AIConnectionTestResult = {
  success?: boolean;
  message?: string;
  latencyMs?: number;
  modelCount?: number;
  models?: string[];
};

type AIPromptTestResult = {
  success?: boolean;
  result?: string;
  durationMs?: number;
};

export const settingsKeys = {
  aiSettings: ['settings', 'ai'] as const,
  section: (section: SettingsSection) => ['settings', 'section', section] as const,
  paths: (collection: 'paths' | 'libraries', kind: PathKind) =>
    ['settings', collection, kind] as const,
  llmProviders: ['settings', 'llm-providers'] as const,
  llmStatus: (providerId: string) => ['settings', 'llm-providers', providerId, 'status'] as const,
};

export function useAISettings() {
  return useQuery<RuntimeAISettings>({
    queryKey: settingsKeys.aiSettings,
    queryFn: () => api.get('/ai/settings'),
    refetchInterval: 30 * 1000,
  });
}

export function useLLMProviders() {
  return useQuery<components['schemas']['LLMProviderInfo'][]>({
    queryKey: settingsKeys.llmProviders,
    queryFn: () => api.get('/llm-providers'),
    refetchInterval: 30 * 1000,
  });
}

export function useLLMProviderStatus(providerId = 'ollama') {
  return useQuery<components['schemas']['LLMProviderStatus']>({
    queryKey: settingsKeys.llmStatus(providerId),
    queryFn: () => api.get(`/llm-providers/${providerId}/status`),
    refetchInterval: 15 * 1000,
  });
}

export function useApplyAISettings() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (payload: {
      enabled?: boolean;
      endpoint?: string;
      primaryModel?: string;
      fallbackModel?: string;
    }) => api.post('/ai/settings/apply', payload),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: settingsKeys.aiSettings });
      queryClient.invalidateQueries({ queryKey: settingsKeys.llmProviders });
      queryClient.invalidateQueries({ queryKey: settingsKeys.llmStatus('ollama') });
    },
  });
}

export function useTestAIConnection() {
  return useMutation({
    mutationFn: (payload: { endpoint?: string }) =>
      api.post<AIConnectionTestResult>('/ai/test-connection', payload),
  });
}

export function useTestAIPrompt() {
  return useMutation({
    mutationFn: (payload: { endpoint?: string; model?: string }) =>
      api.post<AIPromptTestResult>('/ai/test-prompt', payload),
  });
}

type AIModelsResult = {
  success?: boolean;
  endpoint?: string;
  models?: string[];
  message?: string;
};

export function useAIModels(endpoint: string | undefined, enabled = true) {
  return useQuery<AIModelsResult>({
    queryKey: ['settings', 'ai', 'models', endpoint ?? ''],
    queryFn: () => {
      const qs = endpoint ? `?endpoint=${encodeURIComponent(endpoint)}` : '';
      return api.get<AIModelsResult>(`/ai/models${qs}`);
    },
    enabled,
    staleTime: 30_000,
    retry: false,
  });
}

export function useSettingsSection(section: SettingsSection) {
  return useQuery<Record<string, unknown>>({
    queryKey: settingsKeys.section(section),
    queryFn: () => getSettingsSection(section),
  });
}

export function useUpdateSettingsSection(section: SettingsSection) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (payload: Record<string, unknown>) => putSettingsSection(section, payload),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: settingsKeys.section(section) });
      if (section === 'ai') {
        queryClient.invalidateQueries({ queryKey: settingsKeys.aiSettings });
        queryClient.invalidateQueries({ queryKey: settingsKeys.llmStatus('ollama') });
      }
    },
  });
}

export function useSettingsPaths(collection: 'paths' | 'libraries', kind: PathKind) {
  return useQuery<string[]>({
    queryKey: settingsKeys.paths(collection, kind),
    queryFn: async () => {
      const result = await getSettingsPaths(collection, kind);
      return result.paths ?? [];
    },
  });
}

export function useAddSettingsPath(collection: 'paths' | 'libraries', kind: PathKind) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (path: string) => addSettingsPath(collection, kind, path),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: settingsKeys.paths(collection, kind) }),
  });
}

export function useRemoveSettingsPath(collection: 'paths' | 'libraries', kind: PathKind) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (index: number) => removeSettingsPath(collection, kind, index),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: settingsKeys.paths(collection, kind) }),
  });
}

export function useTestSettingsConnection(service: 'sonarr' | 'radarr' | 'jellyfin' | 'jellystat') {
  return useMutation({
    mutationFn: (payload: Record<string, unknown>) => testSettingsConnection(service, payload),
  });
}
