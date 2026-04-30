'use client';

import { useEffect, useState } from 'react';
import { AppShell } from '@/components/layout/AppShell';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Calendar, Play, StopCircle, RotateCw, Check, X, AlertTriangle } from 'lucide-react';
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

export default function SchedulerPage() {
  const [jobs, setJobs] = useState<Job[]>([]);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [counts, setCounts] = useState<Record<string, number>>({});
  const [statusFilter, setStatusFilter] = useState<string>('');
  const [loading, setLoading] = useState(true);

  const refresh = async () => {
    try {
      const [jobsRes, tasksRes] = await Promise.all([
        fetch('/api/v1/scheduler/jobs').then((r) => r.json()),
        fetch(`/api/v1/housekeeping/tasks${statusFilter ? `?status=${statusFilter}` : ''}`).then((r) => r.json()),
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

  const runJob = async (name: string) => {
    const res = await fetch(`/api/v1/scheduler/jobs/${name}/run`, { method: 'POST' });
    if (!res.ok) toast.error(`Failed to run ${name}`);
    else toast.success(`${name} started`);
    refresh();
  };
  const stopJob = async (name: string) => {
    const res = await fetch(`/api/v1/scheduler/jobs/${name}/stop`, { method: 'POST' });
    if (!res.ok) toast.error(`Failed to stop ${name}`);
    else toast.success(`${name} stopped`);
    refresh();
  };
  const updateJob = async (name: string, schedule: string, enabled: boolean) => {
    const res = await fetch(`/api/v1/scheduler/jobs/${name}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ schedule, enabled }),
    });
    if (!res.ok) toast.error('Update failed');
    else toast.success('Updated');
    refresh();
  };
  const retryTask = async (id: number) => {
    await fetch(`/api/v1/housekeeping/tasks/${id}/retry`, { method: 'POST' });
    refresh();
  };
  const cancelTask = async (id: number) => {
    await fetch(`/api/v1/housekeeping/tasks/${id}/cancel`, { method: 'POST' });
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
    if (!res.ok) toast.error('Detect failed');
    else toast.success('Detect started');
    refresh();
  };

  return (
    <AppShell>
      <div className="space-y-6">
        <div className="flex items-center gap-3">
          <Calendar className="h-6 w-6 text-fuchsia-300" />
          <h1 className="text-2xl font-bold">Scheduler</h1>
        </div>

        <Tabs defaultValue="jobs">
          <TabsList>
            <TabsTrigger value="jobs">Scheduled Jobs ({jobs.length})</TabsTrigger>
            <TabsTrigger value="tasks">
              Housekeeping Tasks ({Object.values(counts).reduce((a, b) => a + b, 0)})
            </TabsTrigger>
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
            <div className="flex flex-wrap items-center gap-2">
              <Button size="sm" variant="default" onClick={detectNow} title="Run housekeeping.detect now">
                <Play className="h-3 w-3 mr-1" /> Rescan
              </Button>
              <Button size="sm" variant="outline" onClick={verifyFlagged} title="Run TMDB verifier on every flagged year_mismatch">
                <AlertTriangle className="h-3 w-3 mr-1" /> Re-verify flagged
              </Button>
              <div className="w-px h-5 bg-zinc-700 mx-1" />
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
            <Card>
              <CardContent className="p-0">
                <table className="w-full text-sm">
                  <thead className="text-xs text-zinc-400 border-b border-zinc-800">
                    <tr>
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
                    {tasks.length === 0 && (
                      <tr>
                        <td colSpan={8} className="p-6 text-center text-zinc-500">
                          No tasks.
                        </td>
                      </tr>
                    )}
                    {tasks.map((t) => {
                      const lab = STATUS_LABELS[t.status] ?? { label: t.status, color: 'bg-zinc-700' };
                      const path =
                        (t.payload?.path as string) ||
                        (t.payload?.src_path as string) ||
                        (t.payload?.source as string) ||
                        '';
                      const verification = t.payload?.verification as
                        | { verdict?: string; source?: string; reason?: string }
                        | undefined;
                      const note = verification?.reason || t.last_error || '';
                      return (
                        <tr key={t.id} className="border-b border-zinc-900 hover:bg-zinc-900/40">
                          <td className="p-2 font-mono text-zinc-500">{t.id}</td>
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
                          <td className="p-2 text-right">
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
    </AppShell>
  );
}
