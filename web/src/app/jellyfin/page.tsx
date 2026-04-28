'use client';

import { useState, useMemo, Suspense } from 'react';
import { useSearchParams, useRouter } from 'next/navigation';
import { useQuery } from '@tanstack/react-query';
import { AppShell } from '@/components/layout/AppShell';
import { api } from '@/lib/api/client';
import { useJellyfinIdentification } from '@/hooks/useDashboard';
import { CheckCircle2, HelpCircle, AlertTriangle, Clock } from 'lucide-react';

type Status = 'unidentified' | 'pending' | 'failed' | 'identified';

type IdentificationItem = {
  id: number;
  source_path: string;
  target_path: string;
  parsed_title?: string;
  parsed_year?: number;
  media_type?: string;
  jellyfin_item_id?: string;
  imdb_id?: string;
  tmdb_id?: string;
  tvdb_id?: string;
  resolved_at?: string;
  first_seen_at?: string;
  target_at?: string;
  auto_label?: string;
  identified?: boolean;
};

type ListResponse = {
  status: string;
  count: number;
  items: IdentificationItem[];
};

const TABS: Array<{ key: Status; label: string; icon: any }> = [
  { key: 'unidentified', label: 'Unidentified', icon: HelpCircle },
  { key: 'pending', label: 'Pending (not seen)', icon: Clock },
  { key: 'failed', label: 'Failed', icon: AlertTriangle },
  { key: 'identified', label: 'Identified', icon: CheckCircle2 },
];

function JellyfinPageInner() {
  const params = useSearchParams();
  const router = useRouter();
  const initial = (params.get('status') as Status) || 'unidentified';
  const [status, setStatus] = useState<Status>(initial);

  const { data: stats } = useJellyfinIdentification();

  const { data, isLoading } = useQuery<ListResponse>({
    queryKey: ['jellyfin', 'identification', 'items', status],
    queryFn: () => api.get(`/jellyfin/identification/items?status=${status}&limit=500`),
    refetchInterval: 60 * 1000,
  });

  const counts = useMemo(() => ({
    unidentified: stats?.unidentified ?? 0,
    pending: stats?.pending_no_seen ?? 0,
    failed: stats?.failed_auto_label ?? 0,
    identified: stats?.identified ?? 0,
  }), [stats]);

  function selectTab(s: Status) {
    setStatus(s);
    router.replace(`/jellyfin?status=${s}`, { scroll: false });
  }

  return (
    <AppShell>
      <div className="space-y-6">
        <div>
          <h1 className="text-3xl font-bold">Jellyfin Identification</h1>
          <p className="text-zinc-400 mt-1">
            Drill-down of which moved files Jellyfin has identified, missed, or never seen.
          </p>
        </div>

        <div className="flex flex-wrap gap-2">
          {TABS.map(({ key, label, icon: Icon }) => (
            <button
              key={key}
              onClick={() => selectTab(key)}
              className={`flex items-center gap-2 px-4 py-2 rounded-lg border transition-colors ${
                status === key
                  ? 'bg-zinc-100 text-zinc-900 border-zinc-100'
                  : 'bg-zinc-900 text-zinc-200 border-zinc-800 hover:border-zinc-600'
              }`}
            >
              <Icon className="h-4 w-4" />
              <span>{label}</span>
              <span className="text-xs opacity-70">({counts[key].toLocaleString()})</span>
            </button>
          ))}
        </div>

        <div className="bg-zinc-900 rounded-lg border border-zinc-800 overflow-hidden">
          {isLoading ? (
            <div className="p-6 text-zinc-500">Loading…</div>
          ) : !data?.items?.length ? (
            <div className="p-6 text-zinc-500">No items in this category.</div>
          ) : (
            <table className="w-full text-sm">
              <thead className="bg-zinc-950 text-zinc-400">
                <tr>
                  <th className="text-left p-3">Title</th>
                  <th className="text-left p-3">Type</th>
                  <th className="text-left p-3">Target Path</th>
                  <th className="text-left p-3">IDs</th>
                  <th className="text-left p-3">Last Seen</th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((item) => (
                  <tr key={item.id} className="border-t border-zinc-800 hover:bg-zinc-800/40">
                    <td className="p-3 font-medium">
                      {item.parsed_title || '—'}
                      {item.parsed_year ? ` (${item.parsed_year})` : ''}
                    </td>
                    <td className="p-3 text-zinc-400">{item.media_type || '—'}</td>
                    <td className="p-3 text-zinc-300 font-mono text-xs break-all">
                      {item.target_path}
                    </td>
                    <td className="p-3 text-xs text-zinc-400">
                      {item.imdb_id && <div>imdb:{item.imdb_id}</div>}
                      {item.tmdb_id && <div>tmdb:{item.tmdb_id}</div>}
                      {item.tvdb_id && <div>tvdb:{item.tvdb_id}</div>}
                      {!item.imdb_id && !item.tmdb_id && !item.tvdb_id && '—'}
                    </td>
                    <td className="p-3 text-xs text-zinc-400">
                      {item.resolved_at
                        ? new Date(item.resolved_at).toLocaleString()
                        : item.target_at
                        ? `moved ${new Date(item.target_at).toLocaleString()}`
                        : '—'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
        {data && data.count >= 500 && (
          <p className="text-sm text-zinc-500">Showing first 500 results.</p>
        )}
      </div>
    </AppShell>
  );
}

export default function JellyfinPage() {
  return (
    <Suspense fallback={null}>
      <JellyfinPageInner />
    </Suspense>
  );
}
