'use client';

import { useEffect, useMemo, useState } from 'react';
import { AppShell } from '@/components/layout/AppShell';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Input } from '@/components/ui/input';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { AlertDialog } from '@/components/ui/alert-dialog';
import {
  Calendar,
  Play,
  StopCircle,
  RotateCw,
  Check,
  X,
  AlertTriangle,
  Trash2,
  Search,
  Eye,
  Sparkles,
  Droplets,
} from 'lucide-react';
import { toast } from 'sonner';

type Job = {
  name: string;
  schedule: string;
  enabled: boolean;
  running: boolean;
  last_run_at?: string;
  last_duration_ms?: number;
  last_result?: string;
  last_error?: string;
  next_run_at?: string;
};

type Task = {
  id: number;
  job_name: string;
  kind: string;
  payload: Record<string, unknown>;
  status: string;
  attempts: number;
  priority: number;
  created_at: string;
  last_error?: string;
  started_at?: string;
  finished_at?: string;
};

type DupeFile = {
  id: number;
  path: string;
  size: number;
  resolution: string;
  source_type: string;
  quality_score: number;
  would_keep: boolean;
};

type DupeGroup = {
  group_id?: string;
  media_type?: string;
  title?: string;
  year?: number;
  season?: number;
  episode?: number;
  best_file_id?: number;
  reclaimable_bytes?: number;
  confidence?: string;
  confidence_reasons?: string[];
  files?: DupeFile[];
  resolved?: boolean;
};

const STATUS_LABELS: Record<string, { label: string; color: string }> = {
  pending: { label: 'Pending', color: 'bg-zinc-700 text-zinc-100' },
  running: { label: 'Running', color: 'bg-blue-600 text-white' },
  done: { label: 'Done', color: 'bg-green-600 text-white' },
  failed: { label: 'Failed', color: 'bg-red-600 text-white' },
  skipped: { label: 'Skipped', color: 'bg-zinc-600 text-zinc-100' },
  flagged: { label: 'Flagged', color: 'bg-amber-600 text-white' },
  canceled: { label: 'Canceled', color: 'bg-zinc-500 text-white' },
};

function pathOf(t: Task): string {
  const p = t.payload || {};
  return (
    (p.path as string) ||
    (p.src_path as string) ||
    (p.source as string) ||
    ''
  );
}

function summaryOf(t: Task): string {
  const path = pathOf(t);
  if (path) return path;
  // Duplicate-kind tasks have no path field — show a meaningful
  // human-readable summary instead so the row isn't blank.
  const p = t.payload || {};
  const title =
    (p.normalized_title as string) || (p.title as string) || '';
  if (!title) return '';
  let s = title;
  const yr = p.year;
  if (typeof yr === 'number' && yr > 0) s += ` (${yr})`;
  const season = p.season;
  const ep = p.episode;
  if (typeof season === 'number' && typeof ep === 'number') {
    s += ` S${String(season).padStart(2, '0')}E${String(ep).padStart(2, '0')}`;
  }
  if (typeof p.file_count === 'number') s += ` · ${p.file_count} files`;
  if (typeof p.reclaimable === 'number' && p.reclaimable > 0) {
    s += ` · ${(p.reclaimable / 1024 / 1024 / 1024).toFixed(2)}GB reclaimable`;
  }
  return s;
}

function relativeTime(iso: string): string {
  const ms = Date.now() - new Date(iso).getTime();
  if (Number.isNaN(ms) || ms < 0) return '';
  const s = Math.floor(ms / 1000);
  if (s < 60) return `${s}s ago`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.floor(h / 24);
  return `${d}d ago`;
}

function TaskProgress({ taskId, startedAt }: { taskId: number; startedAt?: string }) {
  const [phase, setPhase] = useState<string>('');
  const [msg, setMsg] = useState<string>('');
  const [current, setCurrent] = useState<number>(0);
  const [total, setTotal] = useState<number>(0);
  const [lastFrameAt, setLastFrameAt] = useState<number>(Date.now());
  const [done, setDone] = useState<boolean>(false);

  useEffect(() => {
    if (typeof EventSource === 'undefined') return;
    const es = new EventSource(`/api/v1/events/op/hk-task-${taskId}`);
    es.onmessage = (ev) => {
      try {
        const f = JSON.parse(ev.data);
        setLastFrameAt(Date.now());
        if (f.type === 'progress') {
          if (f.phase) setPhase(f.phase);
          if (typeof f.msg === 'string') setMsg(f.msg);
          if (typeof f.current === 'number') setCurrent(f.current);
          if (typeof f.total === 'number') setTotal(f.total);
        } else if (f.type === 'done' || f.type === 'error') {
          setDone(true);
          es.close();
        }
      } catch {
        /* ignore */
      }
    };
    es.onerror = () => {
      // server returns 404 if op not yet registered (task just claimed) —
      // browser will auto-retry, harmless.
    };
    return () => es.close();
  }, [taskId]);

  // Stale detector: row marked running but we haven't seen a progress
  // frame in 10 minutes. Either rsync is on a single huge file or the
  // worker is genuinely wedged.
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 5_000);
    return () => clearInterval(t);
  }, []);
  const startedMs = startedAt ? new Date(startedAt).getTime() : now;
  const elapsedSec = Math.max(0, Math.floor((now - startedMs) / 1000));
  const sinceFrameSec = Math.floor((now - lastFrameAt) / 1000);
  const possiblyStuck = sinceFrameSec > 600 && !done;

  const pct = total > 0 ? Math.min(100, Math.round((current / total) * 100)) : 0;

  return (
    <div className="text-xs text-zinc-400 space-y-1">
      <div className="flex items-center gap-2">
        <div className="flex-1 h-1.5 bg-zinc-800 rounded overflow-hidden">
          <div
            className={`h-full transition-all ${
              possiblyStuck ? 'bg-amber-600' : 'bg-blue-500'
            }`}
            style={{ width: `${pct}%` }}
          />
        </div>
        <span className="font-mono w-10 text-right text-zinc-500">{pct}%</span>
      </div>
      <div className="flex items-center gap-2 text-[10px] text-zinc-500">
        {phase && <span className="text-terminal-cyan">{phase}</span>}
        {total > 0 && (
          <span>
            {current}/{total} MB
          </span>
        )}
        <span>· {formatDuration(elapsedSec)}</span>
        {possiblyStuck && (
          <span className="text-amber-400">
            ⚠ no progress {Math.floor(sinceFrameSec / 60)}m
          </span>
        )}
      </div>
      {msg && (
        <div className="truncate text-[10px] text-zinc-600" title={msg}>
          {msg}
        </div>
      )}
    </div>
  );
}

function formatDuration(sec: number): string {
  if (sec < 60) return `${sec}s`;
  const m = Math.floor(sec / 60);
  if (m < 60) return `${m}m ${sec % 60}s`;
  const h = Math.floor(m / 60);
  return `${h}h ${m % 60}m`;
}

export default function SchedulerPage() {
  const [jobs, setJobs] = useState<Job[]>([]);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [counts, setCounts] = useState<Record<string, number>>({});
  const [statusFilter, setStatusFilter] = useState<string>(() => {
    if (typeof window === 'undefined') return '';
    const u = new URL(window.location.href);
    return u.searchParams.get('status') || '';
  });
  const [kindFilter, setKindFilter] = useState<string>('');
  const [search, setSearch] = useState('');
  const [loading, setLoading] = useState(true);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [detail, setDetail] = useState<Task | null>(null);
  const [detailGroup, setDetailGroup] = useState<DupeGroup | null>(null);
  const [limit, setLimit] = useState<number>(200);
  const [confirmAction, setConfirmAction] = useState<null | {
    title: string;
    description: string;
    onConfirm: () => void;
    destructive?: boolean;
  }>(null);

  const refresh = async () => {
    try {
      const params = new URLSearchParams();
      if (statusFilter) params.set('status', statusFilter);
      params.set('limit', String(limit));
      const [jobsRes, tasksRes] = await Promise.all([
        fetch('/api/v1/scheduler/jobs').then((r) => r.json()),
        fetch(`/api/v1/housekeeping/tasks?${params}`).then((r) => r.json()),
      ]);
      setJobs(jobsRes.jobs || []);
      setTasks(tasksRes.tasks || []);
      setCounts(tasksRes.counts || {});
    } catch (err) {
      toast.error('Failed to load scheduler data');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    refresh();
    // Pause polling while a detail dialog is open or the user is editing
    // an input/select. Polling races with typed values and forces React
    // to re-mount inputs mid-keystroke.
    const tick = () => {
      if (detail !== null) return;
      const a = document.activeElement;
      if (a && (a.tagName === 'INPUT' || a.tagName === 'SELECT' || a.tagName === 'TEXTAREA')) {
        return;
      }
      refresh();
    };
    const t = setInterval(tick, 5000);
    return () => clearInterval(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [statusFilter, limit, detail]);

  const filteredTasks = useMemo(() => {
    const q = search.trim().toLowerCase();
    return tasks.filter((t) => {
      if (kindFilter && t.kind !== kindFilter) return false;
      if (!q) return true;
      const reasons = (t.payload?.reasons as string[] | undefined) || [];
      const haystack = [
        String(t.id),
        t.kind,
        t.status,
        summaryOf(t),
        t.last_error || '',
        reasons.join(' '),
      ]
        .join(' ')
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [tasks, kindFilter, search]);

  const allKinds = useMemo(() => {
    const set = new Set<string>();
    tasks.forEach((t) => set.add(t.kind));
    return Array.from(set).sort();
  }, [tasks]);

  // Job ops
  const runJob = async (name: string) => {
    const res = await fetch(`/api/v1/scheduler/jobs/${name}/run`, { method: 'POST' });
    res.ok ? toast.success(`${name} started`) : toast.error(`Failed to run ${name}`);
    refresh();
  };
  const stopJob = async (name: string) => {
    const res = await fetch(`/api/v1/scheduler/jobs/${name}/stop`, { method: 'POST' });
    res.ok ? toast.success(`${name} stopped`) : toast.error(`Failed to stop ${name}`);
    refresh();
  };
  const updateJob = async (name: string, schedule: string, enabled: boolean) => {
    const res = await fetch(`/api/v1/scheduler/jobs/${name}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ schedule, enabled }),
    });
    res.ok ? toast.success('Updated') : toast.error('Update failed');
    refresh();
  };

  // Task ops
  const retryTask = async (id: number) => {
    await fetch(`/api/v1/housekeeping/tasks/${id}/retry`, { method: 'POST' });
    refresh();
  };
  const cancelTask = async (id: number) => {
    await fetch(`/api/v1/housekeeping/tasks/${id}/cancel`, { method: 'POST' });
    refresh();
  };
  const approveTask = async (id: number) => {
    const res = await fetch(`/api/v1/housekeeping/tasks/${id}/approve`, { method: 'POST' });
    if (!res.ok) {
      const txt = await res.text();
      toast.error(`Approve failed: ${txt}`);
      return;
    }
    toast.success('Approved — will execute on next housekeeping tick');
    // Refresh the open detail in place so the badge flips to Pending
    // immediately and the user sees the result without re-opening.
    if (detail && detail.id === id) {
      try {
        const r = await fetch(`/api/v1/housekeeping/tasks/${id}`);
        if (r.ok) setDetail(await r.json());
      } catch (_) { /* best-effort */ }
    }
    refresh();
  };
  const verifyTask = async (id: number) => {
    const res = await fetch(`/api/v1/housekeeping/tasks/${id}/verify`, { method: 'POST' });
    if (!res.ok) {
      toast.error('Verify failed');
      return;
    }
    const v = await res.json();
    toast.success(`Verdict: ${v.Verdict ?? 'unknown'} — ${v.Reason ?? ''}`);
    refresh();
  };
  const verifyFlagged = async () => {
    const res = await fetch('/api/v1/housekeeping/verify-flagged', { method: 'POST' });
    if (!res.ok) {
      const txt = await res.text();
      toast.error(`Re-verify failed: ${txt}`);
      return;
    }
    const data = await res.json();
    toast.success(
      `Verifier: scanned=${data.Scanned ?? 0} distinct=${data.Distinct ?? 0} duplicate=${data.Duplicate ?? 0} unknown=${data.Unknown ?? 0}`
    );
    refresh();
  };
  const detectNow = async () => {
    const res = await fetch('/api/v1/scheduler/jobs/housekeeping.detect/run', { method: 'POST' });
    res.ok ? toast.success('Detect started') : toast.error('Detect failed');
    refresh();
  };
  const drainNow = async () => {
    const res = await fetch('/api/v1/scheduler/jobs/housekeeping.drain/run', { method: 'POST' });
    res.ok ? toast.success('Drain started') : toast.error('Drain failed');
    refresh();
  };

  const openDetail = async (id: number) => {
    const res = await fetch(`/api/v1/housekeeping/tasks/${id}`);
    if (!res.ok) {
      toast.error('Failed to load task');
      return;
    }
    const task: Task = await res.json();
    setDetail(task);
    setDetailGroup(null);
    if (
      task.kind === 'cross_volume_duplicate' ||
      task.kind === 'year_mismatch' ||
      task.kind === 'consolidate_duplicate'
    ) {
      try {
        const gr = await fetch(`/api/v1/housekeeping/tasks/${id}/group`);
        if (gr.ok) setDetailGroup(await gr.json());
      } catch (_) {
        /* group lookup is best-effort */
      }
    }
  };

  // Bulk
  const selectedIds = Array.from(selected);
  const toggleAll = () => {
    if (selected.size === filteredTasks.length && filteredTasks.length > 0) {
      setSelected(new Set());
    } else {
      setSelected(new Set(filteredTasks.map((t) => t.id)));
    }
  };
  const toggleOne = (id: number) => {
    const next = new Set(selected);
    next.has(id) ? next.delete(id) : next.add(id);
    setSelected(next);
  };
  const bulk = async (action: 'retry' | 'cancel' | 'approve') => {
    if (selectedIds.length === 0) return;
    const res = await fetch('/api/v1/housekeeping/tasks/bulk', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ids: selectedIds, action }),
    });
    if (!res.ok) {
      toast.error('Bulk action failed');
      return;
    }
    const data = await res.json();
    toast.success(`${action} affected ${data.affected ?? 0} task(s)`);
    setSelected(new Set());
    refresh();
  };
  const purgeDone = async () => {
    const res = await fetch(
      '/api/v1/housekeeping/tasks?statuses=done,skipped,canceled',
      { method: 'DELETE' }
    );
    if (!res.ok) {
      toast.error('Purge failed');
      return;
    }
    const data = await res.json();
    toast.success(`Deleted ${data.deleted ?? 0} task(s)`);
    refresh();
  };

  const totalTasks = Object.values(counts).reduce((a, b) => a + b, 0);
  const headlineCounts = ['pending', 'running', 'flagged', 'failed'] as const;

  return (
    <AppShell>
      <div className="space-y-6">
        <div className="flex items-center gap-3">
          <Calendar className="h-6 w-6 text-terminal-cyan" />
          <h1 className="text-2xl font-bold">Scheduler</h1>
        </div>

        {/* Counts widget — clickable to filter the task list */}
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
          {headlineCounts.map((s) => {
            const lab = STATUS_LABELS[s];
            const active = statusFilter === s;
            return (
              <Card
                key={s}
                onClick={() => setStatusFilter(active ? '' : s)}
                className={`cursor-pointer transition-colors ${
                  active ? 'border-terminal-cyan/60 bg-terminal-cyan/5' : 'hover:border-zinc-700'
                }`}
              >
                <CardContent className="p-4">
                  <div className="text-xs text-zinc-400 uppercase">{lab.label}</div>
                  <div className="text-2xl font-semibold">{counts[s] ?? 0}</div>
                </CardContent>
              </Card>
            );
          })}
        </div>

        <Tabs defaultValue="jobs">
          <TabsList>
            <TabsTrigger value="jobs">Scheduled Jobs ({jobs.length})</TabsTrigger>
            <TabsTrigger value="tasks">Housekeeping Tasks ({totalTasks})</TabsTrigger>
          </TabsList>

          <TabsContent value="jobs" className="space-y-3">
            {loading && <div className="text-zinc-400">Loading…</div>}
            {jobs.length === 0 && !loading && (
              <Card>
                <CardContent className="py-6 text-zinc-400">No scheduled jobs registered yet.</CardContent>
              </Card>
            )}
            {jobs.map((j) => (
              <Card key={j.name}>
                <CardHeader className="flex flex-row items-center justify-between">
                  <CardTitle className="flex items-center gap-2 text-lg">
                    {j.name}
                    {j.running && <Badge className="bg-blue-600">running</Badge>}
                    {!j.enabled && <Badge variant="secondary">disabled</Badge>}
                  </CardTitle>
                  <div className="flex gap-2">
                    {!j.running ? (
                      <Button size="sm" variant="outline" onClick={() => runJob(j.name)}>
                        <Play className="h-3 w-3 mr-1" /> Run now
                      </Button>
                    ) : (
                      <Button size="sm" variant="destructive" onClick={() => stopJob(j.name)}>
                        <StopCircle className="h-3 w-3 mr-1" /> Stop
                      </Button>
                    )}
                  </div>
                </CardHeader>
                <CardContent className="space-y-2 text-sm">
                  <div className="flex items-center gap-2">
                    <span className="text-zinc-400">Schedule:</span>
                    <input
                      defaultValue={j.schedule}
                      onBlur={(e) => {
                        if (e.target.value !== j.schedule) updateJob(j.name, e.target.value, j.enabled);
                      }}
                      className="bg-zinc-900/40 border border-zinc-700/50 rounded px-2 py-1 text-sm font-mono transition-colors focus:outline-none focus:ring-2 focus:ring-amber-500/40 focus:border-amber-500/30"
                    />
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => updateJob(j.name, j.schedule, !j.enabled)}
                    >
                      {j.enabled ? 'Disable' : 'Enable'}
                    </Button>
                  </div>
                  {j.last_run_at && (
                    <div className="text-zinc-400">
                      Last: {new Date(j.last_run_at).toLocaleString()} ({j.last_duration_ms}ms){' '}
                      {j.last_result && <span className="text-zinc-300">— {j.last_result}</span>}
                    </div>
                  )}
                  {j.last_error && (
                    <div className="flex items-start gap-2 text-red-400">
                      <AlertTriangle className="h-4 w-4 mt-0.5" />
                      <span>{j.last_error}</span>
                    </div>
                  )}
                  {j.next_run_at && (
                    <div className="text-zinc-500">Next: {new Date(j.next_run_at).toLocaleString()}</div>
                  )}
                </CardContent>
              </Card>
            ))}
          </TabsContent>

          <TabsContent value="tasks" className="space-y-3">
            {/* Action toolbar */}
            <div className="flex flex-wrap items-center gap-2">
              <Button size="sm" variant="default" onClick={detectNow} title="Run housekeeping.detect now">
                <Play className="h-3 w-3 mr-1" /> Rescan
              </Button>
              <Button size="sm" variant="outline" onClick={drainNow} title="Run housekeeping.drain now">
                <Droplets className="h-3 w-3 mr-1" /> Drain now
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={verifyFlagged}
                title="Run TMDB verifier on every flagged year_mismatch"
              >
                <Sparkles className="h-3 w-3 mr-1" /> Re-verify flagged
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={() =>
                  setConfirmAction({
                    title: 'Clear completed tasks?',
                    description:
                      'Permanently deletes all done/skipped/canceled tasks from the queue. Active flagged/pending tasks are not affected.',
                    destructive: true,
                    onConfirm: purgeDone,
                  })
                }
                title="Delete done/skipped/canceled tasks"
              >
                <Trash2 className="h-3 w-3 mr-1" /> Clear completed
              </Button>
              <div className="w-px h-5 bg-zinc-700 mx-1" />
              {/* Status filter chips */}
              <Button
                size="sm"
                variant={statusFilter === '' ? 'default' : 'outline'}
                onClick={() => setStatusFilter('')}
              >
                All
              </Button>
              {Object.entries(counts).map(([s, n]) => (
                <Button
                  key={s}
                  size="sm"
                  variant={statusFilter === s ? 'default' : 'outline'}
                  onClick={() => setStatusFilter(s)}
                >
                  {STATUS_LABELS[s]?.label ?? s} ({n})
                </Button>
              ))}
            </div>

            {/* Search + kind filter */}
            <div className="flex flex-wrap items-center gap-2">
              <div className="relative flex-1 min-w-[240px]">
                <Search className="h-4 w-4 absolute left-2 top-1/2 -translate-y-1/2 text-zinc-500" />
                <Input
                  className="pl-8"
                  placeholder="Search by id, title, path, kind…"
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                />
              </div>
              <select
                value={kindFilter}
                onChange={(e) => setKindFilter(e.target.value)}
                className="bg-zinc-900/40 border border-zinc-700/50 rounded px-2 py-1 text-sm transition-colors focus:outline-none focus:ring-2 focus:ring-amber-500/40 focus:border-amber-500/30"
              >
                <option value="">All kinds</option>
                {allKinds.map((k) => (
                  <option key={k} value={k}>
                    {k}
                  </option>
                ))}
              </select>
            </div>

            {/* Bulk action bar (only when selection) */}
            {selectedIds.length > 0 && (
              <div className="flex items-center gap-2 bg-zinc-900/60 border border-terminal-amber/30 rounded px-3 py-2 text-sm">
                <span className="text-terminal-amber font-medium">{selectedIds.length} selected</span>
                <div className="flex-1" />
                <Button
                  size="sm"
                  variant="default"
                  onClick={() => bulk('approve')}
                  className="bg-emerald-600 hover:bg-emerald-500"
                  title="Promote flagged duplicates to auto-delete queue"
                >
                  <Check className="h-3 w-3 mr-1" /> Approve flagged
                </Button>
                <Button size="sm" variant="outline" onClick={() => bulk('retry')} title="Reset to pending">
                  <RotateCw className="h-3 w-3 mr-1" /> Retry
                </Button>
                <Button
                  size="sm"
                  variant="destructive"
                  onClick={() =>
                    setConfirmAction({
                      title: `Cancel ${selectedIds.length} task(s)?`,
                      description: 'Pending and flagged tasks will be marked canceled.',
                      destructive: true,
                      onConfirm: () => bulk('cancel'),
                    })
                  }
                >
                  <X className="h-3 w-3 mr-1" /> Cancel
                </Button>
                <Button size="sm" variant="ghost" onClick={() => setSelected(new Set())}>
                  Clear
                </Button>
              </div>
            )}

            <Card>
              <CardContent className="p-0">
                <table className="w-full text-sm">
                  <thead className="text-xs text-zinc-400 border-b border-zinc-800">
                    <tr>
                      <th className="p-2 w-8">
                        <input
                          type="checkbox"
                          checked={
                            filteredTasks.length > 0 && selected.size === filteredTasks.length
                          }
                          onChange={toggleAll}
                          className="accent-terminal-cyan"
                        />
                      </th>
                      <th className="p-2 text-left">ID</th>
                      <th className="p-2 text-left">Kind</th>
                      <th className="p-2 text-left">Status</th>
                      <th className="p-2 text-left">Item</th>
                      <th className="p-2 text-left">Notes</th>
                      <th className="p-2 text-left">Attempts</th>
                      <th className="p-2 text-left">Created</th>
                      <th className="p-2 text-right">Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {filteredTasks.length === 0 && (
                      <tr>
                        <td colSpan={9} className="p-6 text-center text-zinc-500">
                          {tasks.length === 0
                            ? 'No tasks.'
                            : `No tasks match the current filter${
                                kindFilter ? ` (kind=${kindFilter})` : ''
                              }${search ? ` (search="${search}")` : ''}.`}
                        </td>
                      </tr>
                    )}
                    {filteredTasks.map((t) => {
                      const lab = STATUS_LABELS[t.status] ?? { label: t.status, color: 'bg-zinc-700' };
                      const path = summaryOf(t);
                      const verification = t.payload?.verification as
                        | { verdict?: string; source?: string; reason?: string }
                        | undefined;
                      const reasonsArr = (t.payload?.reasons as string[] | undefined) || [];
                      const note =
                        verification?.reason ||
                        t.last_error ||
                        (reasonsArr.length > 0 ? reasonsArr.join(' · ') : '');
                      const isSelected = selected.has(t.id);
                      return (
                        <tr
                          key={t.id}
                          className={`border-b border-zinc-900 hover:bg-zinc-900/40 ${
                            isSelected ? 'bg-terminal-amber/10' : ''
                          }`}
                        >
                          <td className="p-2">
                            <input
                              type="checkbox"
                              checked={isSelected}
                              onChange={() => toggleOne(t.id)}
                              className="accent-terminal-cyan"
                            />
                          </td>
                          <td className="p-2 font-mono text-zinc-500">
                            <button
                              className="hover:text-terminal-cyan"
                              onClick={() => openDetail(t.id)}
                              title="View details"
                            >
                              {t.id}
                            </button>
                          </td>
                          <td className="p-2">{t.kind}</td>
                          <td className="p-2">
                            <Badge className={lab.color}>{lab.label}</Badge>
                          </td>
                          <td className="p-2 text-xs text-zinc-400 truncate max-w-md" title={path}>
                            {path}
                          </td>
                          <td className="p-2 text-xs text-zinc-500 truncate max-w-xs" title={note}>
                            {t.status === 'running' ? (
                              <TaskProgress taskId={t.id} startedAt={t.started_at} />
                            ) : (
                              <>
                                {verification?.source && (
                                  <span className="text-terminal-cyan">[{verification.source}] </span>
                                )}
                                {note}
                              </>
                            )}
                          </td>
                          <td className="p-2">{t.attempts}</td>
                          <td className="p-2 text-zinc-500 text-xs" title={new Date(t.created_at).toLocaleString()}>
                            {relativeTime(t.created_at)}
                          </td>
                          <td className="p-2 text-right whitespace-nowrap">
                            <Button
                              size="sm"
                              variant="ghost"
                              onClick={() => openDetail(t.id)}
                              title="View details"
                            >
                              <Eye className="h-3 w-3" />
                            </Button>
                            {t.status === 'flagged' && t.kind === 'year_mismatch' && (
                              <Button
                                size="sm"
                                variant="ghost"
                                onClick={() => verifyTask(t.id)}
                                title="Re-verify with TMDB"
                              >
                                <Sparkles className="h-3 w-3 text-terminal-cyan" />
                              </Button>
                            )}
                            {t.status === 'flagged' && (
                              <Button
                                size="sm"
                                variant="ghost"
                                onClick={() => {
                                  if (
                                    t.kind === 'cross_volume_duplicate' ||
                                    t.kind === 'year_mismatch'
                                  ) {
                                    approveTask(t.id);
                                  } else {
                                    retryTask(t.id);
                                  }
                                }}
                                title="Approve and execute"
                              >
                                <Check className="h-3 w-3 text-emerald-500" />
                              </Button>
                            )}
                            {(t.status === 'failed' ||
                              t.status === 'canceled' ||
                              t.status === 'skipped') && (
                              <Button
                                size="sm"
                                variant="ghost"
                                onClick={() => retryTask(t.id)}
                                title="Retry"
                              >
                                <RotateCw className="h-3 w-3" />
                              </Button>
                            )}
                            {(t.status === 'pending' || t.status === 'flagged') && (
                              <Button
                                size="sm"
                                variant="ghost"
                                onClick={() => cancelTask(t.id)}
                                title={t.status === 'flagged' ? 'Dismiss' : 'Cancel'}
                              >
                                <X className="h-3 w-3" />
                              </Button>
                            )}
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </CardContent>
            </Card>

            {/* Pagination footer: server returns up to `limit` rows but
                counts reflects the full DB. Show how much we are seeing
                vs how much exists, and a Load more button. */}
            <div className="flex items-center justify-between text-xs text-zinc-500 px-1">
              <div>
                Showing <span className="text-zinc-300">{filteredTasks.length}</span>
                {filteredTasks.length !== tasks.length && (
                  <> of <span className="text-zinc-300">{tasks.length}</span> loaded</>
                )}
                {totalTasks > tasks.length && (
                  <> · <span className="text-zinc-400">{totalTasks - tasks.length} older not loaded</span></>
                )}
                {totalTasks > 0 && <> · {totalTasks} total in queue</>}
              </div>
              <div className="flex items-center gap-2">
                {totalTasks > tasks.length && (
                  <Button size="sm" variant="outline" onClick={() => setLimit((l) => l + 200)}>
                    Load 200 more
                  </Button>
                )}
                {tasks.length > 200 && (
                  <Button size="sm" variant="ghost" onClick={() => setLimit(200)}>
                    Reset
                  </Button>
                )}
              </div>
            </div>
          </TabsContent>
        </Tabs>
      </div>

      {/* Task detail modal */}
      <Dialog open={detail !== null} onOpenChange={(o) => { if (!o) { setDetail(null); setDetailGroup(null); } }}>
        <DialogContent className="bg-zinc-950/90 backdrop-blur-xl border-amber-500/10 max-w-3xl">
          <DialogHeader>
            <DialogTitle>
              Task #{detail?.id} — {detail?.kind}
            </DialogTitle>
          </DialogHeader>
          {detail && (
            <div className="space-y-3 text-sm max-h-[70vh] overflow-y-auto">
              <div className="flex flex-wrap gap-2 text-xs">
                <Badge className={STATUS_LABELS[detail.status]?.color ?? 'bg-zinc-700'}>
                  {STATUS_LABELS[detail.status]?.label ?? detail.status}
                </Badge>
                <Badge variant="secondary">attempts: {detail.attempts}</Badge>
                <Badge variant="secondary">priority: {detail.priority}</Badge>
                <Badge variant="secondary">job: {detail.job_name}</Badge>
              </div>
              <div className="text-xs text-zinc-500">
                Created: {new Date(detail.created_at).toLocaleString()}
                {detail.started_at && ` · Started: ${new Date(detail.started_at).toLocaleString()}`}
                {detail.finished_at && ` · Finished: ${new Date(detail.finished_at).toLocaleString()}`}
              </div>
              {detail.last_error && (
                <div className="rounded bg-red-950/40 border border-red-900 px-3 py-2 text-red-200 text-xs whitespace-pre-wrap">
                  {detail.last_error}
                </div>
              )}
              <div>
                <div className="text-xs text-zinc-400 mb-1">Payload</div>
                <pre className="bg-black/40 border border-zinc-800 rounded p-3 text-xs overflow-x-auto">
                  {JSON.stringify(detail.payload, null, 2)}
                </pre>
              </div>
              {detailGroup && !detailGroup.resolved && detailGroup.files && (
                <div>
                  <div className="flex items-center justify-between mb-2">
                    <div className="text-xs text-zinc-400">
                      Duplicate group ({detailGroup.files.length} files
                      {detailGroup.confidence ? ` · ${detailGroup.confidence} confidence` : ''})
                    </div>
                    {(detail.kind === 'cross_volume_duplicate' ||
                      detail.kind === 'year_mismatch') &&
                      detail.status === 'flagged' && (
                        <Button
                          size="sm"
                          onClick={() => {
                            approveTask(detail.id);
                            setDetail(null);
                            setDetailGroup(null);
                          }}
                          className="bg-emerald-600 hover:bg-emerald-500"
                        >
                          <Check className="h-3 w-3 mr-1" /> Approve & queue delete
                        </Button>
                      )}
                  </div>
                  {detailGroup.confidence_reasons && detailGroup.confidence_reasons.length > 0 && (
                    <div className="text-xs text-amber-300 mb-2">
                      {detailGroup.confidence_reasons.join(' · ')}
                    </div>
                  )}
                  <table className="w-full text-xs border border-zinc-800 rounded">
                    <thead className="bg-zinc-900/60 text-zinc-400">
                      <tr>
                        <th className="p-2 text-left">Action</th>
                        <th className="p-2 text-left">Path</th>
                        <th className="p-2 text-right">Size</th>
                        <th className="p-2 text-left">Res</th>
                        <th className="p-2 text-left">Source</th>
                        <th className="p-2 text-right">Score</th>
                      </tr>
                    </thead>
                    <tbody>
                      {detailGroup.files.map((f) => (
                        <tr
                          key={f.id}
                          className={f.would_keep ? 'bg-emerald-950/30' : 'bg-red-950/20'}
                        >
                          <td className="p-2 whitespace-nowrap">
                            {f.would_keep ? (
                              <span className="text-emerald-400">KEEP</span>
                            ) : (
                              <span className="text-red-400">DELETE</span>
                            )}
                          </td>
                          <td className="p-2 font-mono break-all">{f.path}</td>
                          <td className="p-2 text-right whitespace-nowrap">
                            {(f.size / 1024 / 1024).toFixed(0)} MB
                          </td>
                          <td className="p-2">{f.resolution || '-'}</td>
                          <td className="p-2">{f.source_type || '-'}</td>
                          <td className="p-2 text-right">{f.quality_score}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                  {detailGroup.reclaimable_bytes && detailGroup.reclaimable_bytes > 0 && (
                    <div className="text-xs text-zinc-500 mt-2">
                      Reclaimable: {(detailGroup.reclaimable_bytes / 1024 / 1024 / 1024).toFixed(2)} GB
                    </div>
                  )}
                </div>
              )}
              {detailGroup && detailGroup.resolved && (
                <div className="text-xs text-zinc-500 italic">
                  Duplicate group already resolved (files removed or merged).
                </div>
              )}
            </div>
          )}
        </DialogContent>
      </Dialog>

      {/* Confirm dialog */}
      {confirmAction && (
        <AlertDialog
          open={true}
          onOpenChange={(o) => !o && setConfirmAction(null)}
          title={confirmAction.title}
          description={confirmAction.description}
          destructive={confirmAction.destructive}
          confirmLabel="Confirm"
          onConfirm={confirmAction.onConfirm}
        />
      )}
    </AppShell>
  );
}
