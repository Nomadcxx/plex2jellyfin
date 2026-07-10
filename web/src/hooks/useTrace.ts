import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api/client';

export type TraceJellyfin = {
  item_id?: string;
  imdb_id?: string;
  tmdb_id?: string;
  tvdb_id?: string;
  resolved_at?: string;
  identified?: boolean;
};

export type TraceItem = {
  id: number;
  source_path: string;
  source_filename: string;
  event_at: string;
  media_type?: string;
  parse_method?: string;
  parsed_title?: string;
  parsed_year?: number;
  parsed_season?: number;
  parsed_episode?: number;
  stripped_tokens?: string;
  target_path?: string;
  target_at?: string;
  outcome?: string;
  error?: string;
  jellyfin?: TraceJellyfin;
  label?: string;
};

type TraceResponse = { items: TraceItem[] };

export function useTrace(query: string, limit = 50) {
  return useQuery<TraceResponse>({
    queryKey: ['trace', query, limit],
    queryFn: () =>
      api.get(`/files/trace?q=${encodeURIComponent(query)}&limit=${limit}`),
    refetchInterval: 15000,
  });
}
