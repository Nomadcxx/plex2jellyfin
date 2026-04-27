import { useEffect } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api/client';
import { components } from '@/types/api';

type ActivityResponse = {
  events: components['schemas']['ActivityEvent'][];
  total: number;
};

export const activityKeys = {
  all: ['activity'] as const,
};

export function useActivity() {
  const queryClient = useQueryClient();

  // Live invalidation via the SSE stream so new events appear without
  // waiting for the polling interval. Falls back gracefully when the
  // stream is unavailable (the polling refetchInterval still runs).
  useEffect(() => {
    if (typeof window === 'undefined' || typeof EventSource === 'undefined') return;
    const es = new EventSource('/api/v1/activity/stream');
    const onActivity = () => {
      queryClient.invalidateQueries({ queryKey: activityKeys.all });
    };
    es.addEventListener('activity', onActivity);
    return () => {
      es.removeEventListener('activity', onActivity);
      es.close();
    };
  }, [queryClient]);

  return useQuery<ActivityResponse>({
    queryKey: activityKeys.all,
    queryFn: () => api.get('/activity'),
    refetchInterval: 10000,
  });
}
