import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api/client';

export const queryKeys = {
  dashboard: ['dashboard'] as const,
  mediaManagers: ['media-managers'] as const,
  duplicates: ['duplicates'] as const,
  scattered: ['scattered'] as const,
  auth: ['auth', 'status'] as const,
};

export function useDashboard() {
  return useQuery({
    queryKey: queryKeys.dashboard,
    queryFn: () => api.get('/dashboard'),
    refetchInterval: 30 * 1000,
  });
}

export function useMediaManagers() {
  return useQuery({
    queryKey: queryKeys.mediaManagers,
    queryFn: () => api.get('/media-managers'),
  });
}

export function useDuplicates() {
  return useQuery({
    queryKey: queryKeys.duplicates,
    queryFn: () => api.get('/duplicates'),
    refetchInterval: 60 * 1000,
  });
}

export function useScattered() {
  return useQuery({
    queryKey: queryKeys.scattered,
    queryFn: () => api.get('/scattered'),
  });
}

export function useAuthStatus() {
  return useQuery({
    queryKey: queryKeys.auth,
    queryFn: () => api.get('/auth/status'),
  });
}
