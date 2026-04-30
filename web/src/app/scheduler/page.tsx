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

function titleOf(t: Task): string {
  const p = t.payload || {};
  return (p.title as string) || '';
}

export default function SchedulerPage() {
  const [jobs, setJobs] = useState<Job[]>([]);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [counts, setCounts] = useState<Record<string, number>>({});
  const [statusFilter, setStatusFilter] = useState<string>('');
  const [kindFilter, setKindFilter] = useState<string>('');
  const [search, setSearch] = useState('');
  const [loading, setLoading] = useState(true);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [detail, setDetail] = useState<Task | null>(null);
  const [confirmAction, setConfirmAction] = useState<null | {
    title: string;
    description: string;
    onConfirm: () => void;
    destructive?: boolean;
  }>(null);

  const refresh = async () => {
    try {
      const [jobsRes, tasksRes] = await Promise.all([
        fetch('/api/v1/scheduler/jobs').then((r) => r.json()),
        fetch(`/api/v1/housekeeping/tasks${statusFilter ? `?status=${statusFilter}` : ''}`).then((r) =>
          r.json()
        ),
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
    const t = setInterval(refresh, 5000);
    return () => clearInterval(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [statusFilter]);

  const filteredTasks = useMemo(() => {
    const q = search.trim().toLowerCase();
    return tasks.filter((t) => {
      if (kindFilter && t.kind !== kindFilter) return false;
      if (!q) return true;
      const haystack = `${t.id} ${t.kind} ${titleOf(t)} ${pathOf(t)}`.toLowerCase();
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
    setDetail(await res.json());
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
  const bulk = async (action: 'retry' | 'cancel') => {
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
          <Calendar className="h-6 w-6 text-fuchsia-300" />
          <h1 className="text-2xl font-bold">Scheduler</h1>
        </div>

        {/* Counts widget */}
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
          {headlineCounts.map((s) => {
            const lab = STATUS_LABELS[s];
            return (
              <Card key={s}>
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
                      className="bg-zinc-900 border border-zinc-700 rounded px-2 py-1 text-sm font-mono"
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
                className="bg-zinc-900 border border-zinc-700 rounded px-2 py-1 text-sm"
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
              <div className="flex items-center gap-2 bg-zinc-900 border border-fuchsia-800/40 rounded px-3 py-2 text-sm">
                <span className="text-fuchsia-300 font-medium">{selectedIds.length} selected</span>
                <div className="flex-1" />
                <Button size="sm" variant="default" onClick={() => bulk('retry')}>
                  <Check className="h-3 w-3 mr-1" /> Approve / retry
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
                          className="accent-fuchsia-500"
                        />
                      </th>
                      <th className="p-2 text-left">ID</th>
                      <th className="p-2 text-left">Kind</th>
                      <th className="p-2 text-left">Status</th>
                      <th className="p-2 text-left">Path</th>
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
                          No tasks.
                        </td>
                      </tr>
                    )}
                    {filteredTasks.map((t) => {
                      const lab = STATUS_LABELS[t.status] ?? { label: t.status, color: 'bg-zinc-700' };
                      const path = pathOf(t);
                      const verification = t.payload?.verification as
                        | { verdict?: string; source?: string; reason?: string }
                        | undefined;
                      const note = verification?.reason || t.last_error || '';
                      const isSelected = selected.has(t.id);
                      return (
                        <tr
                          key={t.id}
                          className={`border-b border-zinc-900 hover:bg-zinc-900/40 ${
                            isSelected ? 'bg-fuchsia-950/30' : ''
                          }`}
                        >
                          <td className="p-2">
                            <input
                              type="checkbox"
                              checked={isSelected}
                              onChange={() => toggleOne(t.id)}
                              className="accent-fuchsia-500"
                            />
                          </td>
                          <td className="p-2 font-mono text-zinc-500">
                            <button
                              className="hover:text-fuchsia-300"
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
                            {verification?.source && (
                              <span className="text-fuchsia-400">[{verification.source}] </span>
                            )}
                            {note}
                          </td>
                          <td className="p-2">{t.attempts}</td>
                          <td className="p-2 text-zinc-500 text-xs">
                            {new Date(t.created_at).toLocaleString()}
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
                                <Sparkles className="h-3 w-3 text-fuchsia-300" />
                              </Button>
                            )}
                            {t.status === 'flagged' && (
                              <Button
                                size="sm"
                                variant="ghost"
                                onClick={() => retryTask(t.id)}
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
          </TabsContent>
        </Tabs>
      </div>

      {/* Task detail modal */}
      <Dialog open={detail !== null} onOpenChange={(o) => !o && setDetail(null)}>
        <DialogContent className="bg-zinc-900 border-zinc-800 max-w-3xl">
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
