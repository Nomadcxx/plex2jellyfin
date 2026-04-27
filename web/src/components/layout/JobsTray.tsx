'use client';

import { useEffect, useMemo, useState } from 'react';
import { Activity, Loader2, X } from 'lucide-react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { api } from '@/lib/api/client';

interface OpSummary {
  id: string;
  cmd: string;
  started_at: number;
  state: string;
  phase?: string;
  msg?: string;
  current?: number;
  total?: number;
  pct?: number;
  final_code?: string;
  final_msg?: string;
  finished_at?: number;
}

interface OpsResponse {
  ops: OpSummary[];
}

const POLL_INTERVAL_MS = 4000;

const PRETTY_CMD: Record<string, string> = {
  RESCAN: 'Library rescan',
  RESET_DB: 'Database reset',
  CONSOLIDATE: 'Consolidate series',
  DUP_SCAN: 'Duplicate scan',
  AI_BATCH: 'AI batch enhance',
  METADATA_REFRESH: 'Metadata refresh',
  SWEEP: 'Jellyfin sweep',
  PARSES_AUDIT: 'Parses audit',
};

function prettyCmd(c: string): string {
  return PRETTY_CMD[c] ?? c;
}

function formatPhase(op: OpSummary): string {
  if (op.state !== 'running') {
    return op.state === 'done'
      ? 'Completed'
      : op.state === 'cancelled'
      ? 'Cancelled'
      : op.final_msg
      ? `Failed — ${op.final_msg}`
      : 'Failed';
  }
  if (op.phase && op.msg) return `${op.phase}: ${op.msg}`;
  if (op.phase) return op.phase;
  if (op.msg) return op.msg;
  return 'In progress…';
}

export function JobsTray() {
  const [open, setOpen] = useState(false);
  const queryClient = useQueryClient();

  const { data, refetch } = useQuery<OpsResponse>({
    queryKey: ['ops'],
    queryFn: () => api.get<OpsResponse>('/ops'),
    refetchInterval: POLL_INTERVAL_MS,
    staleTime: POLL_INTERVAL_MS,
    retry: false,
  });

  const ops = useMemo(() => data?.ops ?? [], [data]);
  const running = ops.filter((o) => o.state === 'running');

  // Auto-close when nothing is running and panel was just emptied.
  useEffect(() => {
    if (running.length === 0 && open && ops.length === 0) {
      setOpen(false);
    }
  }, [running.length, ops.length, open]);

  const cancel = async (id: string) => {
    try {
      await api.post(`/ops/${id}/cancel`, {});
      toast.success('Cancellation requested');
      queryClient.invalidateQueries({ queryKey: ['ops'] });
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Cancel failed');
    }
  };

  return (
    <div className="relative">
      <Button
        variant="outline"
        size="sm"
        onClick={() => {
          setOpen((o) => !o);
          refetch();
        }}
        className="gap-2"
        aria-label={`Active jobs (${running.length} running)`}
      >
        {running.length > 0 ? (
          <Loader2 className="h-4 w-4 animate-spin text-sky-400" />
        ) : (
          <Activity className="h-4 w-4" />
        )}
        Jobs
        {running.length > 0 && (
          <span className="inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-sky-500 px-1.5 text-[11px] font-semibold text-zinc-950">
            {running.length}
          </span>
        )}
      </Button>

      {open && (
        <div className="absolute right-0 top-full z-40 mt-2 w-96 max-h-[70vh] overflow-y-auto rounded-lg border border-zinc-800 bg-zinc-950 shadow-2xl">
          <div className="flex items-center justify-between border-b border-zinc-800 px-4 py-2">
            <h3 className="text-sm font-semibold">Active Jobs</h3>
            <button
              type="button"
              onClick={() => setOpen(false)}
              className="text-zinc-500 hover:text-zinc-200"
              aria-label="Close jobs panel"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
          {ops.length === 0 ? (
            <p className="px-4 py-6 text-center text-sm text-zinc-500">No recent jobs.</p>
          ) : (
            <ul className="divide-y divide-zinc-800">
              {ops.map((op) => {
                const pct = op.total && op.total > 0
                  ? Math.min(100, Math.round((op.current ?? 0) / op.total * 100))
                  : op.state === 'done' ? 100 : 0;
                return (
                  <li key={op.id} className="px-4 py-3 space-y-1.5">
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0">
                        <p className="text-sm font-medium text-zinc-200 truncate">
                          {prettyCmd(op.cmd)}
                        </p>
                        <p className="text-xs text-zinc-500 truncate">{formatPhase(op)}</p>
                      </div>
                      {op.state === 'running' && (
                        <Button
                          variant="outline"
                          size="sm"
                          className="h-7 px-2 text-xs"
                          onClick={() => cancel(op.id)}
                        >
                          Cancel
                        </Button>
                      )}
                    </div>
                    {op.state === 'running' && op.total ? (
                      <div className="space-y-0.5">
                        <div className="h-1.5 w-full overflow-hidden rounded-full bg-zinc-800">
                          <div
                            className="h-full bg-sky-500 transition-all"
                            style={{ width: `${pct}%` }}
                          />
                        </div>
                        <p className="text-[11px] text-zinc-500">
                          {op.current ?? 0} / {op.total} ({pct}%)
                        </p>
                      </div>
                    ) : op.state === 'running' ? (
                      <div className="h-1.5 w-full overflow-hidden rounded-full bg-zinc-800">
                        <div className="h-full w-1/3 animate-pulse bg-sky-500/60" />
                      </div>
                    ) : null}
                  </li>
                );
              })}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
