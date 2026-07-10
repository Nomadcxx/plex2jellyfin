import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
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
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (draft: SetupDraft) => api.post<{ applied: boolean; complete: boolean; daemon_state: string }>('/setup/apply', draft),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: setupKeys.status }),
  });
}
