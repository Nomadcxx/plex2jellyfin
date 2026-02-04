import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api/client';
import { components } from '@/types/api';

export const queueKeys = {
  all: ['queue'] as const,
  manager: (id: string) => [...queueKeys.all, id] as const,
};

export function useQueue(managerId: string) {
  return useQuery<{ items: components['schemas']['QueueItem'][] }>({
    queryKey: queueKeys.manager(managerId),
    queryFn: () => api.get(`/media-managers/${managerId}/queue`),
    refetchInterval: 5000,
    enabled: !!managerId,
  });
}

export function useStuckItems(managerId: string) {
  return useQuery<{ items: components['schemas']['QueueItem'][] }>({
    queryKey: [...queueKeys.manager(managerId), 'stuck'],
    queryFn: () => api.get(`/media-managers/${managerId}/stuck`),
    refetchInterval: 10000,
    enabled: !!managerId,
  });
}
