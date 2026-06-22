'use client';

import { useState, useMemo, Suspense } from 'react';
import { useSearchParams, useRouter } from 'next/navigation';
import { useQuery } from '@tanstack/react-query';
import { AppShell } from '@/components/layout/AppShell';
import { api } from '@/lib/api/client';
import {
  queryKeys,
  useJellyfinIdentification,
  useReconcileJellyfinMetadata,
  useRepairJellyfinMetadataItem,
  useRepairVisibleJellyfinMetadata,
} from '@/hooks/useDashboard';
import { CheckCircle2, HelpCircle, AlertTriangle, Clock, Info, RefreshCw, Wrench } from 'lucide-react';

type Status = 'unidentified' | 'pending' | 'failed' | 'identified';

type MetadataState =
  | 'identified'
  | 'missing_provider_ids'
  | 'missing_episode_numbers'
  | 'series_unidentified'
  | 'series_identified_episode_stale'
  | 'jellyfin_item_missing'
  | 'path_mismatch'
  | 'recent_import_waiting'
  | 'needs_review'
  | '';

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
  metadata_state?: MetadataState;
  metadata_error?: string;
  metadata_check_count?: number;
  metadata_repair_count?: number;
  last_metadata_check_at?: string;
  next_metadata_check_at?: string;
  last_metadata_repair_at?: string;
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

const METADATA_STATE_OPTIONS: Array<{ value: MetadataState | 'all'; label: string }> = [
  { value: 'all', label: 'All metadata states' },
  { value: '', label: 'Unknown' },
  { value: 'identified', label: 'Identified' },
  { value: 'missing_provider_ids', label: 'Missing provider IDs' },
  { value: 'missing_episode_numbers', label: 'Missing episode numbers' },
  { value: 'series_unidentified', label: 'Series unidentified' },
  { value: 'series_identified_episode_stale', label: 'Episode stale' },
  { value: 'jellyfin_item_missing', label: 'Jellyfin item missing' },
  { value: 'path_mismatch', label: 'Path mismatch' },
  { value: 'recent_import_waiting', label: 'Waiting for Jellyfin' },
  { value: 'needs_review', label: 'Needs review' },
];

const METADATA_STATE_LABELS: Record<MetadataState, string> = {
  '': 'Unknown',
  identified: 'Identified',
  missing_provider_ids: 'Missing provider IDs',
  missing_episode_numbers: 'Missing episode numbers',
  series_unidentified: 'Series unidentified',
  series_identified_episode_stale: 'Episode stale',
  jellyfin_item_missing: 'Jellyfin item missing',
  path_mismatch: 'Path mismatch',
  recent_import_waiting: 'Waiting for Jellyfin',
  needs_review: 'Needs review',
};

const METADATA_STATE_STYLES: Record<MetadataState, string> = {
  '': 'border-zinc-700 bg-zinc-800 text-zinc-300',
  identified: 'border-emerald-800 bg-emerald-950/40 text-emerald-300',
  missing_provider_ids: 'border-amber-800 bg-amber-950/40 text-amber-300',
  missing_episode_numbers: 'border-amber-800 bg-amber-950/40 text-amber-300',
  series_unidentified: 'border-orange-800 bg-orange-950/40 text-orange-300',
  series_identified_episode_stale: 'border-orange-800 bg-orange-950/40 text-orange-300',
  jellyfin_item_missing: 'border-red-800 bg-red-950/40 text-red-300',
  path_mismatch: 'border-red-800 bg-red-950/40 text-red-300',
  recent_import_waiting: 'border-sky-800 bg-sky-950/40 text-sky-300',
  needs_review: 'border-fuchsia-800 bg-fuchsia-950/40 text-fuchsia-300',
};

function formatDateTime(value?: string) {
  return value ? new Date(value).toLocaleString() : '—';
}

function metadataDetails(item: IdentificationItem) {
  const parts = [
    item.metadata_error ? `Error: ${item.metadata_error}` : null,
    item.last_metadata_check_at ? `Last check: ${formatDateTime(item.last_metadata_check_at)}` : null,
    item.last_metadata_repair_at ? `Last repair: ${formatDateTime(item.last_metadata_repair_at)}` : null,
  ].filter(Boolean);

  return parts.length ? parts.join('\n') : 'No metadata recovery details yet';
}

function JellyfinPageInner() {
  const params = useSearchParams();
  const router = useRouter();
  const initial = (params.get('status') as Status) || 'unidentified';
  const [status, setStatus] = useState<Status>(initial);
  const [metadataState, setMetadataState] = useState<MetadataState | 'all'>('all');

  const { data: stats } = useJellyfinIdentification();
  const reconcileMutation = useReconcileJellyfinMetadata(status);
  const repairItemMutation = useRepairJellyfinMetadataItem(status);

  const { data, isLoading } = useQuery<ListResponse>({
    queryKey: queryKeys.jellyfinIdentificationItems(status),
    queryFn: () => api.get(`/jellyfin/identification/items?status=${status}&limit=500`),
    refetchInterval: 60 * 1000,
  });

  const counts = useMemo(() => ({
    unidentified: stats?.unidentified ?? 0,
    pending: stats?.pending_no_seen ?? 0,
    failed: stats?.failed_auto_label ?? 0,
    identified: stats?.identified ?? 0,
  }), [stats]);

  const visibleItems = useMemo(() => {
    const items = data?.items ?? [];
    if (metadataState === 'all') {
      return items;
    }
    return items.filter((item) => (item.metadata_state ?? '') === metadataState);
  }, [data?.items, metadataState]);

  const repairVisibleDecisionIds = useMemo(() => visibleItems.slice(0, 5).map((item) => item.id), [visibleItems]);
  const repairVisibleMutation = useRepairVisibleJellyfinMetadata(status, repairVisibleDecisionIds);

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

        <div className="flex flex-wrap items-center gap-3">
          <select
            value={metadataState}
            onChange={(event) => setMetadataState(event.target.value as MetadataState | 'all')}
            className="h-10 rounded-lg border border-zinc-800 bg-zinc-900 px-3 text-sm text-zinc-100 outline-none transition-colors hover:border-zinc-600 focus:border-zinc-500"
          >
            {METADATA_STATE_OPTIONS.map((option) => (
              <option key={option.value || 'unknown'} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
          <button
            type="button"
            onClick={() => reconcileMutation.mutate()}
            disabled={reconcileMutation.isPending}
            className="flex h-10 items-center gap-2 rounded-lg border border-zinc-800 bg-zinc-900 px-4 text-sm text-zinc-200 transition-colors hover:border-zinc-600 disabled:cursor-not-allowed disabled:opacity-60"
          >
            <RefreshCw className={`h-4 w-4 ${reconcileMutation.isPending ? 'animate-spin' : ''}`} />
            <span>Reconcile</span>
          </button>
          <button
            type="button"
            onClick={() => repairVisibleMutation.mutate()}
            disabled={repairVisibleMutation.isPending || repairVisibleDecisionIds.length === 0}
            className="flex h-10 items-center gap-2 rounded-lg border border-zinc-800 bg-zinc-900 px-4 text-sm text-zinc-200 transition-colors hover:border-zinc-600 disabled:cursor-not-allowed disabled:opacity-60"
          >
            <Wrench className="h-4 w-4" />
            <span>Repair visible rows</span>
          </button>
        </div>

        <div className="bg-zinc-900 rounded-lg border border-zinc-800 overflow-hidden">
          {isLoading ? (
            <div className="p-6 text-zinc-500">Loading…</div>
          ) : !visibleItems.length ? (
            <div className="p-6 text-zinc-500">No items in this category.</div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full min-w-[1120px] text-sm">
                <thead className="bg-zinc-950 text-zinc-400">
                  <tr>
                    <th className="text-left p-3">Title</th>
                    <th className="text-left p-3">Type</th>
                    <th className="text-left p-3">Target Path</th>
                    <th className="text-left p-3">IDs</th>
                    <th className="text-left p-3">Last Seen</th>
                    <th className="text-left p-3">Metadata State</th>
                    <th className="text-left p-3">Checks / Repairs</th>
                    <th className="text-left p-3">Next Check</th>
                    <th className="text-right p-3">Action</th>
                  </tr>
                </thead>
                <tbody>
                  {visibleItems.map((item) => {
                    const state = item.metadata_state ?? '';
                    const repairingThisRow = repairItemMutation.isPending && repairItemMutation.variables === item.id;

                    return (
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
                            ? formatDateTime(item.resolved_at)
                            : item.target_at
                            ? `moved ${formatDateTime(item.target_at)}`
                            : '—'}
                        </td>
                        <td className="p-3">
                          <div className="flex items-center gap-2">
                            <span className={`inline-flex whitespace-nowrap rounded-full border px-2 py-1 text-xs ${METADATA_STATE_STYLES[state]}`}>
                              {METADATA_STATE_LABELS[state]}
                            </span>
                            {(item.metadata_error || item.last_metadata_check_at || item.last_metadata_repair_at) && (
                              <span title={metadataDetails(item)}>
                                <Info className="h-4 w-4 text-zinc-500" aria-hidden="true" />
                              </span>
                            )}
                          </div>
                        </td>
                        <td className="p-3 text-xs text-zinc-400">
                          {(item.metadata_check_count ?? 0).toLocaleString()} / {(item.metadata_repair_count ?? 0).toLocaleString()}
                        </td>
                        <td className="p-3 text-xs text-zinc-400">{formatDateTime(item.next_metadata_check_at)}</td>
                        <td className="p-3 text-right">
                          <button
                            type="button"
                            onClick={() => repairItemMutation.mutate(item.id)}
                            disabled={repairingThisRow || repairItemMutation.isPending}
                            className="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-zinc-800 bg-zinc-950 text-zinc-300 transition-colors hover:border-zinc-600 hover:text-zinc-100 disabled:cursor-not-allowed disabled:opacity-60"
                            title={`Repair metadata for ${item.parsed_title || item.target_path}`}
                          >
                            <Wrench className={`h-4 w-4 ${repairingThisRow ? 'animate-pulse' : ''}`} />
                          </button>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
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
