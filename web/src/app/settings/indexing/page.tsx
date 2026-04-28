'use client';

import { useEffect, useState } from 'react';
import { toast } from 'sonner';
import { Database, FolderTree, Loader2, RefreshCw, Search, Trash2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { ConfirmDestructive } from '@/components/settings/ConfirmDestructive';
import { ProgressCard } from '@/components/settings/ProgressCard';
import { useOpStream } from '@/hooks/useOpStream';
import { api } from '@/lib/api/client';

type LastEntry = {
  id: string;
  cmd: string;
  args?: { paths?: string[]; dry_run?: boolean };
  state: string;
  msg?: string;
  started_at: string;
  ended_at?: string;
};

type LastResponse = { entries: LastEntry[] | null };

type ScanScope = 'all' | 'tv' | 'movies' | 'watch';

function durationSeconds(a?: string, b?: string): number {
  if (!a || !b) return 0;
  return Math.max(0, (new Date(b).getTime() - new Date(a).getTime()) / 1000);
}

function fmtDuration(s: number): string {
  if (s <= 0) return '—';
  if (s < 60) return `${Math.round(s)}s`;
  const m = Math.floor(s / 60);
  const r = Math.round(s % 60);
  if (m < 60) return `${m}m ${r}s`;
  const h = Math.floor(m / 60);
  return `${h}h ${m % 60}m`;
}

function fmtRelative(iso?: string): string {
  if (!iso) return 'never';
  const d = new Date(iso);
  const ago = (Date.now() - d.getTime()) / 1000;
  if (ago < 60) return 'just now';
  if (ago < 3600) return `${Math.round(ago / 60)} min ago`;
  if (ago < 86400) return `${Math.round(ago / 3600)} h ago`;
  return d.toLocaleString();
}

function stateBadge(state: string) {
  const cls =
    state === 'done'
      ? 'bg-green-500/15 text-green-300 border-green-700/40'
      : state === 'error'
        ? 'bg-red-500/15 text-red-300 border-red-700/40'
        : state === 'cancelled'
          ? 'bg-yellow-500/15 text-yellow-300 border-yellow-700/40'
          : 'bg-zinc-500/15 text-zinc-300 border-zinc-700/40';
  return (
    <span className={`inline-flex items-center rounded border px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide ${cls}`}>
      {state}
    </span>
  );
}

export default function IndexingPage() {
  const [opID, setOpID] = useState<string | null>(null);
  const [confirmReset, setConfirmReset] = useState(false);
  const [isStarting, setIsStarting] = useState(false);
  const [last, setLast] = useState<LastEntry[]>([]);
  const [loadingLast, setLoadingLast] = useState(false);
  const { events } = useOpStream(opID);

  const lastEvent = events[events.length - 1];
  const opFinished =
    lastEvent?.type === 'done' ||
    lastEvent?.type === 'error' ||
    lastEvent?.type === 'cancelled';

  const refreshLast = async () => {
    setLoadingLast(true);
    try {
      const data = await api.get<LastResponse>('/database/rescan/last');
      setLast(data.entries ?? []);
    } catch (err) {
      console.warn('failed to load last scans', err);
    } finally {
      setLoadingLast(false);
    }
  };

  useEffect(() => {
    refreshLast();
  }, []);

  useEffect(() => {
    if (opFinished) refreshLast();
  }, [opFinished]);

  async function startRescan(scope: ScanScope, dryRun = false) {
    setIsStarting(true);
    try {
      let paths: string[] = [];
      if (scope === 'all') {
        paths = [];
      } else if (scope === 'tv') {
        const cfg = await api.get<{ paths?: string[] }>('/settings/libraries/tv').catch(() => null);
        paths = cfg?.paths ?? [];
      } else if (scope === 'movies') {
        const cfg = await api.get<{ paths?: string[] }>('/settings/libraries/movies').catch(() => null);
        paths = cfg?.paths ?? [];
      } else if (scope === 'watch') {
        const [tv, mv] = await Promise.all([
          api.get<{ paths?: string[] }>('/settings/paths/tv').catch(() => null),
          api.get<{ paths?: string[] }>('/settings/paths/movies').catch(() => null),
        ]);
        paths = [...(tv?.paths ?? []), ...(mv?.paths ?? [])];
      }
      if (scope !== 'all' && paths.length === 0) {
        throw new Error(`No paths configured for ${scope}; set them in Settings → Paths/Libraries.`);
      }

      const r = await fetch('/api/v1/database/rescan', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ paths, dry_run: dryRun }),
      });
      if (!r.ok) throw new Error(`Server error ${r.status}`);
      const { op_id } = await r.json();
      setOpID(op_id);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to start re-scan');
    } finally {
      setIsStarting(false);
    }
  }

  async function startReset() {
    setConfirmReset(false);
    setIsStarting(true);
    try {
      const r = await fetch('/api/v1/database/reset', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ confirm: 'media.db', preserve: ['audit_log'] }),
      });
      if (!r.ok) throw new Error(`Server error ${r.status}`);
      const { op_id } = await r.json();
      setOpID(op_id);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to start reset');
    } finally {
      setIsStarting(false);
    }
  }

  const lastSuccess = last.find((e) => e.state === 'done');
  const mostRecent = last[0];

  return (
    <div className="space-y-6">
      <div className="flex items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">Indexing &amp; Scanning</h1>
          <p className="text-sm text-muted-foreground">
            Walk libraries and watch folders to refresh the media index. Long scans
            stream live progress and ETA — leave this tab open or check the global Jobs tray.
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={refreshLast} disabled={loadingLast} className="gap-1">
          <RefreshCw className={`h-3.5 w-3.5 ${loadingLast ? 'animate-spin' : ''}`} />
          Refresh
        </Button>
      </div>

      {opID && (
        <div className="space-y-3">
          <ProgressCard title="Active scan" events={events} />
          {opFinished && (
            <Button variant="outline" onClick={() => setOpID(null)}>
              Dismiss
            </Button>
          )}
        </div>
      )}

      {!opID && (
        <div className="rounded border border-zinc-800 bg-zinc-950/40 p-4">
          <h2 className="font-semibold mb-1 flex items-center gap-2">
            <Search className="h-4 w-4" /> Start a scan
          </h2>
          <p className="text-sm text-muted-foreground mb-3">
            Walks the chosen folders, indexes new files, refreshes parse decisions,
            and updates <span className="font-mono">media.db</span>.
          </p>
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
            <Button onClick={() => startRescan('all')} disabled={isStarting} className="justify-start gap-2">
              {isStarting ? <Loader2 className="h-4 w-4 animate-spin" /> : <FolderTree className="h-4 w-4" />}
              Scan all libraries (write)
            </Button>
            <Button onClick={() => startRescan('tv')} disabled={isStarting} className="justify-start gap-2">
              <FolderTree className="h-4 w-4" /> Scan TV libraries (write)
            </Button>
            <Button onClick={() => startRescan('movies')} disabled={isStarting} className="justify-start gap-2">
              <FolderTree className="h-4 w-4" /> Scan movie libraries (write)
            </Button>
            <Button onClick={() => startRescan('watch')} disabled={isStarting} className="justify-start gap-2">
              <FolderTree className="h-4 w-4" /> Scan watch folders (write)
            </Button>
          </div>
          <div className="mt-3 border-t border-zinc-800 pt-3">
            <Button variant="outline" onClick={() => startRescan('all', true)} disabled={isStarting} className="w-full justify-start gap-2">
              <Search className="h-4 w-4" /> Dry run — walk + report only, no DB writes
            </Button>
          </div>
        </div>
      )}

      <div className="rounded border border-zinc-800 bg-zinc-950/40 p-4">
        <h2 className="font-semibold mb-3 flex items-center gap-2">
          <Database className="h-4 w-4" /> Last scans
        </h2>
        {last.length === 0 ? (
          <p className="text-sm text-muted-foreground">No scan history yet.</p>
        ) : (
          <>
            {lastSuccess && (
              <div className="mb-3 rounded bg-zinc-900 p-3 text-sm">
                <p>
                  <span className="text-zinc-400">Last successful scan:</span>{' '}
                  <span className="font-medium">{fmtRelative(lastSuccess.ended_at)}</span>
                  {' · '}
                  <span className="text-zinc-400">Duration:</span>{' '}
                  <span className="font-medium">
                    {fmtDuration(durationSeconds(lastSuccess.started_at, lastSuccess.ended_at))}
                  </span>
                </p>
              </div>
            )}
            <ul className="divide-y divide-zinc-800 text-sm">
              {last.slice(0, 10).map((e) => (
                <li key={e.id} className="flex items-start justify-between gap-3 py-2">
                  <div className="min-w-0">
                    <p className="flex items-center gap-2">
                      {stateBadge(e.state)}
                      <span className="text-zinc-300">{fmtRelative(e.started_at)}</span>
                      {e.args?.dry_run ? (
                        <span className="rounded bg-zinc-800 px-1.5 py-0.5 text-[10px] uppercase">dry-run</span>
                      ) : null}
                    </p>
                    {e.msg ? <p className="text-xs text-muted-foreground truncate">{e.msg}</p> : null}
                    {e.args?.paths && e.args.paths.length > 0 ? (
                      <p className="text-xs text-muted-foreground truncate">
                        {e.args.paths.length} path{e.args.paths.length === 1 ? '' : 's'}: {e.args.paths.slice(0, 3).join(', ')}
                        {e.args.paths.length > 3 ? '…' : ''}
                      </p>
                    ) : null}
                  </div>
                  <p className="shrink-0 text-xs text-zinc-500 tabular-nums">
                    {fmtDuration(durationSeconds(e.started_at, e.ended_at))}
                  </p>
                </li>
              ))}
            </ul>
          </>
        )}
        <p className="mt-3 text-xs text-muted-foreground">
          Most recent: {mostRecent ? fmtRelative(mostRecent.started_at) : 'never'}.
        </p>
      </div>

      <div className="rounded border border-destructive/30 bg-zinc-950/40 p-4">
        <h2 className="font-semibold mb-1 flex items-center gap-2">
          <Trash2 className="h-4 w-4 text-destructive" /> Reset database
        </h2>
        <p className="text-sm text-muted-foreground">
          Permanently deletes indexed media, parse decisions, and operator overrides.
          Audit log is preserved.
        </p>
        <div className="mt-3">
          <Button variant="destructive" onClick={() => setConfirmReset(true)} disabled={isStarting || !!opID}>
            Reset database…
          </Button>
        </div>
      </div>

      <ConfirmDestructive
        open={confirmReset}
        phrase="media.db"
        onConfirm={startReset}
        onCancel={() => setConfirmReset(false)}
      >
        This will delete every table except <span className="font-mono">audit_log</span>.
      </ConfirmDestructive>
    </div>
  );
}
