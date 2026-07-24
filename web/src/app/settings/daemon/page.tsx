'use client';

import { useState } from 'react';
import Link from 'next/link';
import { Loader2, RefreshCw, Scan } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { useDaemon } from '@/hooks/useDaemon';
import { ConfirmReversible } from '@/components/settings/ConfirmReversible';
import {
  checkArrCompatibility,
  CompatibilityResult,
  getHealth,
  getSettingsSection,
} from '@/lib/api/client';

function formatUptime(seconds: number | undefined): string {
  if (seconds == null) return '—';
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const parts: string[] = [];
  if (d > 0) parts.push(`${d}d`);
  if (h > 0) parts.push(`${h}h`);
  parts.push(`${m}m`);
  return parts.join(' ');
}

function StatusDot({ state }: { state: string }) {
  const color =
    state === 'running' ? 'bg-emerald-500' :
    state === 'interrupted' ? 'bg-amber-500' :
    'bg-rose-500';
  return <span className={`inline-block h-2.5 w-2.5 rounded-full ${color}`} />;
}

type ArrSummary = Partial<Record<'sonarr' | 'radarr', CompatibilityResult | { error: string }>>;

export default function DaemonPage() {
  const { status, action } = useDaemon(5000);
  const [confirmAction, setConfirmAction] = useState<'stop' | 'restart' | null>(null);
  const [healthLine, setHealthLine] = useState<string | null>(null);
  const [arrSummary, setArrSummary] = useState<ArrSummary | null>(null);
  const [checking, setChecking] = useState(false);

  const runHealthAndArr = async () => {
    setChecking(true);
    try {
      const health = await getHealth();
      setHealthLine(`${health.status}${health.version ? ` · ${health.version}` : ''}`);

      const summary: ArrSummary = {};
      for (const service of ['sonarr', 'radarr'] as const) {
        try {
          const section = await getSettingsSection(service);
          if (section.enabled !== true || !section.url || !section.api_key) {
            summary[service] = { error: 'not configured' };
            continue;
          }
          summary[service] = await checkArrCompatibility(service, {
            url: section.url,
            api_key: section.api_key,
          });
        } catch (err) {
          summary[service] = { error: err instanceof Error ? err.message : 'check failed' };
        }
      }
      setArrSummary(summary);
      toast.success('Health checks finished');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Health check failed');
    } finally {
      setChecking(false);
    }
  };

  if (!status) {
    return (
      <div className="flex items-center gap-2 text-sm text-zinc-400">
        <Loader2 className="h-4 w-4 animate-spin" />
        Loading…
      </div>
    );
  }

  if (status.state === 'interrupted') {
    return (
      <div className="space-y-4">
        <h1 className="text-2xl font-semibold">Daemon</h1>
        <div className="rounded border border-destructive/50 bg-destructive/10 p-4">
          <p className="font-semibold">A previous destructive op was interrupted.</p>
          <pre className="mt-2 text-xs">{JSON.stringify(status.interrupted_op, null, 2)}</pre>
          <div className="mt-3 flex gap-2">
            <Button onClick={async () => {
              try {
                const r = await fetch('/api/v1/daemon/recover', {
                  method: 'POST', headers: { 'Content-Type': 'application/json' },
                  body: JSON.stringify({ action: 'discard' }),
                });
                if (!r.ok) throw new Error(`Server error ${r.status}`);
                location.reload();
              } catch (err) {
                toast.error(err instanceof Error ? err.message : 'Discard failed');
              }
            }}>Discard</Button>
            <Button variant="outline" onClick={async () => {
              try {
                const r = await fetch('/api/v1/daemon/recover', {
                  method: 'POST', headers: { 'Content-Type': 'application/json' },
                  body: JSON.stringify({ action: 'resume' }),
                });
                if (!r.ok) throw new Error(`Server error ${r.status}`);
                location.reload();
              } catch (err) {
                toast.error(err instanceof Error ? err.message : 'Resume failed');
              }
            }}>Resume</Button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">Daemon</h1>
      <Card>
        <CardContent className="pt-6 space-y-2">
          <p className="flex items-center gap-2">
            <StatusDot state={status.state} />
            <span className="text-sm text-zinc-400">State:</span>
            <span className="font-mono text-sm">{status.state}</span>
          </p>
          {status.version && (
            <p className="text-sm">
              <span className="text-zinc-400">Version:</span>{' '}
              <span className="font-mono">{status.version}</span>
            </p>
          )}
          <p className="text-sm">
            <span className="text-zinc-400">Uptime:</span>{' '}
            <span className="font-mono">{formatUptime(status.uptime_seconds)}</span>
          </p>
          <div className="mt-4 flex flex-wrap gap-2">
            <Button onClick={async () => {
              try {
                await action('reload');
                toast.success('Daemon reloaded');
              } catch {
                toast.error('Reload failed');
              }
            }}>Reload from disk</Button>
            <Button variant="outline" onClick={() => setConfirmAction('restart')}>Restart</Button>
            <Button variant="outline" onClick={() => setConfirmAction('stop')}>Stop</Button>
          </div>
        </CardContent>
      </Card>

      <div className="space-y-3 border border-zinc-800 bg-zinc-950/40 p-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="font-mono text-sm font-semibold text-zinc-200">Scan & health</h2>
            <p className="mt-1 text-sm text-zinc-500">
              Library rescans live under Indexing; API and Arr health can be re-checked here.
            </p>
          </div>
          <div className="flex gap-2">
            <Link
              href="/settings/indexing"
              className="inline-flex h-9 items-center justify-center gap-2 rounded-md border border-zinc-700 bg-transparent px-3 text-sm font-medium hover:bg-zinc-800"
            >
              <Scan className="h-4 w-4" />
              Open indexing
            </Link>
            <Button type="button" variant="outline" size="sm" disabled={checking} onClick={runHealthAndArr}>
              {checking ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
              Check health
            </Button>
          </div>
        </div>

        {healthLine && (
          <p className="text-sm">
            <span className="text-zinc-400">API:</span>{' '}
            <span className="font-mono text-emerald-300">{healthLine}</span>
          </p>
        )}

        {arrSummary && (
          <ul className="space-y-1 text-sm">
            {(['sonarr', 'radarr'] as const).map((service) => {
              const row = arrSummary[service];
              if (!row) return null;
              if ('error' in row && row.error && !('ok' in row)) {
                return (
                  <li key={service}>
                    <span className="font-mono text-zinc-300">{service}</span>
                    <span className="text-zinc-500"> — {row.error}</span>
                  </li>
                );
              }
              const result = row as CompatibilityResult;
              if (result.error && !result.ok) {
                return (
                  <li key={service}>
                    <span className="font-mono text-zinc-300">{service}</span>
                    <span className="text-rose-300"> — {result.error}</span>
                  </li>
                );
              }
              const n = result.issues?.length ?? 0;
              return (
                <li key={service}>
                  <span className="font-mono text-zinc-300">{service}</span>
                  <span className={result.healthy ? 'text-emerald-300' : 'text-amber-200'}>
                    {' '}— {result.healthy ? 'healthy' : `${n} issue${n === 1 ? '' : 's'}`}
                  </span>
                  {!result.healthy && n > 0 && (
                    <Link href={`/settings/${service}`} className="ml-2 text-xs text-zinc-400 underline">
                      review
                    </Link>
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </div>

      <ConfirmReversible
        open={confirmAction !== null}
        title={`Confirm daemon ${confirmAction}`}
        onConfirm={async () => {
          if (confirmAction) {
            try {
              await action(confirmAction);
              toast.success(`Daemon ${confirmAction} succeeded`);
            } catch {
              toast.error(`Daemon ${confirmAction} failed`);
            }
          }
          setConfirmAction(null);
        }}
        onCancel={() => setConfirmAction(null)}
      >
        Are you sure you want to {confirmAction} the daemon?
      </ConfirmReversible>
    </div>
  );
}
