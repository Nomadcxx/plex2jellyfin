import { useMutation, useQuery } from '@tanstack/react-query';
import type { components } from '@/types/api';
import { api } from '@/lib/api/client';

export type SetupStatus = components['schemas']['SetupStatus'];
export type SetupDraft = components['schemas']['SetupDraft'];

export const setupKeys = {
  all: ['setup'] as const,
  status: ['setup', 'status'] as const,
};

export function useSetupStatus(enabled = true) {
  return useQuery<SetupStatus>({
    queryKey: setupKeys.status,
    queryFn: () => api.get<SetupStatus>('/setup/status'),
    enabled,
    staleTime: 0,
  });
}

export function useApplySetup() {
  return useMutation({
    mutationFn: (draft: SetupDraft) =>
      api.post<{
        applied: boolean;
        complete: boolean;
        indexing?: boolean;
        daemon_state: string;
        plugin_warning?: string;
        scan_warning?: string;
      }>('/setup/apply', draft),
  });
}

export type SetupIndexEvent = {
  type: 'progress' | 'status' | 'done' | 'error';
  phase?: string;
  msg?: string;
  library?: string;
  libraries_done?: number;
  libraries_total?: number;
  files_scanned?: number;
  files_added?: number;
  files_updated?: number;
  files_skipped?: number;
  episode_rows?: number;
  movie_rows?: number;
  duration_ms?: number;
  scan_warning?: string;
  daemon_state?: string;
};
