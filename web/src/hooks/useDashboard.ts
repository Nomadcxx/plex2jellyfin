import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api/client';
import type { components } from '@/types/api';

type DuplicateAnalysis = components['schemas']['DuplicateAnalysis'];

type DashboardResponse = {
  libraryStats?: {
    totalFiles?: number;
    totalSize?: number;
    duplicateGroups?: number;
    scatteredSeries?: number;
  };
  mediaManagers?: Array<{
    id: string;
    name: string;
    type: string;
    online?: boolean;
  }>;
};

type MediaManagerInfo = {
  id: string;
  name: string;
  type: string;
  enabled: boolean;
  online?: boolean;
};

type ScatteredAnalysis = {
  items?: Array<{
    id: number;
    title: string;
    year?: number;
    mediaType: string;
    locations: string[];
    targetLocation?: string;
    filesToMove?: number;
    bytesToMove?: number;
  }>;
  totalItems?: number;
  totalMoves?: number;
  totalBytes?: number;
};

type AuthStatus = {
  enabled: boolean;
  authenticated?: boolean;
};

export const queryKeys = {
  dashboard: ['dashboard'] as const,
  mediaManagers: ['media-managers'] as const,
  duplicates: ['duplicates'] as const,
  scattered: ['scattered'] as const,
  auth: ['auth', 'status'] as const,
};

export function useDashboard() {
  return useQuery<DashboardResponse>({
    queryKey: queryKeys.dashboard,
    queryFn: () => api.get('/dashboard'),
    refetchInterval: 30 * 1000,
  });
}

export function useMediaManagers() {
  return useQuery<MediaManagerInfo[]>({
    queryKey: queryKeys.mediaManagers,
    queryFn: () => api.get('/media-managers'),
  });
}

export function useDuplicates() {
  return useQuery<DuplicateAnalysis>({
    queryKey: queryKeys.duplicates,
    queryFn: () => api.get('/duplicates'),
    refetchInterval: 60 * 1000,
  });
}

export function useScattered() {
  return useQuery<ScatteredAnalysis>({
    queryKey: queryKeys.scattered,
    queryFn: () => api.get('/scattered'),
  });
}

export function useAuthStatus() {
  return useQuery<AuthStatus>({
    queryKey: queryKeys.auth,
    queryFn: () => api.get('/auth/status'),
  });
}

export function useDeleteDuplicate() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ groupId, fileId }: { groupId: string; fileId?: number }) =>
      api.delete(`/duplicates/${groupId}${fileId ? `?fileId=${fileId}` : ''}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.duplicates });
      queryClient.invalidateQueries({ queryKey: queryKeys.dashboard });
    },
  });
}

export function useClearQueueItem(managerId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ itemId, blocklist = false }: { itemId: number; blocklist?: boolean }) =>
      api.delete(`/media-managers/${managerId}/queue/${itemId}?blocklist=${blocklist}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['media-managers', managerId, 'queue'] });
      queryClient.invalidateQueries({ queryKey: queryKeys.mediaManagers });
    },
  });
}
