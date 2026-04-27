import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api/client';
import { components } from '@/types/api';

type ScatteredResponse = {
  items: components['schemas']['ScatteredItem'][];
  totalItems: number;
  totalMoves: number;
  totalBytes: number;
};

export type ConsolidationMove = {
  from: string;
  to: string;
  bytes: number;
};

export type ConsolidationPreview = {
  success: boolean;
  dryRun: boolean;
  filesMoved: number;
  bytesMoved: number;
  targetPath: string;
  moves: ConsolidationMove[];
};

export const consolidationKeys = {
  all: ['consolidation'] as const,
  scattered: ['scattered'] as const,
};

export function useScattered() {
  return useQuery<ScatteredResponse>({
    queryKey: consolidationKeys.scattered,
    queryFn: () => api.get('/scattered'),
  });
}

export function useConsolidateItem() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (itemId: number) =>
      api.post(`/scattered/${itemId}/consolidate`, { dryRun: false }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: consolidationKeys.scattered });
    },
  });
}

export function usePreviewConsolidation() {
  return useMutation<ConsolidationPreview, Error, number>({
    mutationFn: (itemId: number) =>
      api.post(`/scattered/${itemId}/consolidate`, { dryRun: true }),
  });
}
