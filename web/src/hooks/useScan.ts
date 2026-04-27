import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api/client';
import { components } from '@/types/api';

export const scanKeys = {
  all: ['scan'] as const,
  status: ['scan', 'status'] as const,
};

export function useScanStatus() {
  return useQuery<components['schemas']['ScanStatus']>({
    queryKey: scanKeys.status,
    queryFn: () => api.get('/scan/status'),
    refetchInterval: 2000,
  });
}

export function useStartScan() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => api.post('/scan'),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: scanKeys.status });
    },
  });
}
