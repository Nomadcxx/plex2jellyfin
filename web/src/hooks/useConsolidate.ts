import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api/client';
import { components } from '@/types/api';
import { queryKeys } from './useDashboard';

type ConsolidationResult = components['schemas']['ConsolidationResult'];

export function useConsolidate(itemId: number) {
  const queryClient = useQueryClient();
  
  return useMutation({
    mutationFn: ({ dryRun = false }: { dryRun?: boolean }) => 
      api.post<ConsolidationResult>(`/scattered/${itemId}/consolidate`, { dryRun }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.scattered });
      queryClient.invalidateQueries({ queryKey: queryKeys.dashboard });
    },
  });
}
