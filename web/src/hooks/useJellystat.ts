import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api/client';

// Jellystat rows are passed through as-is; key casing differs between
// Jellystat versions, so consumers read defensively via mostViewedName/Plays.
export type JellystatRow = Record<string, unknown>;

export type JellystatOverview = {
  enabled: boolean;
  error?: string;
  libraries?: JellystatRow[];
  most_viewed_movies?: JellystatRow[];
  most_viewed_series?: JellystatRow[];
};

export function useJellystatOverview() {
  return useQuery<JellystatOverview>({
    queryKey: ['jellystat', 'overview'],
    queryFn: () => api.get('/jellystat/overview'),
    staleTime: 60_000,
    refetchInterval: 5 * 60_000,
  });
}

export function mostViewedName(row: JellystatRow): string {
  for (const key of ['Name', 'name', 'NowPlayingItemName', 'ItemName']) {
    const v = row[key];
    if (typeof v === 'string' && v) return v;
  }
  return 'Unknown';
}

export function mostViewedPlays(row: JellystatRow): number | undefined {
  for (const key of ['Plays', 'plays', 'unique_viewers', 'times_played']) {
    const v = row[key];
    if (typeof v === 'number') return v;
    if (typeof v === 'string' && v !== '' && !Number.isNaN(Number(v))) return Number(v);
  }
  return undefined;
}
