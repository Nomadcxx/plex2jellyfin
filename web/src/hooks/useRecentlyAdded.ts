import { useQuery } from '@tanstack/react-query';
import { getRecentlyAdded, RecentlyAddedResponse } from '@/lib/api/client';

export function useRecentlyAdded(limit = 24) {
  return useQuery<RecentlyAddedResponse>({
    queryKey: ['jellyfin', 'recently-added', limit],
    queryFn: () => getRecentlyAdded(limit),
    staleTime: 60_000,
    refetchInterval: 5 * 60_000,
  });
}
