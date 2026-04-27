'use client';

import { useState } from 'react';
import { Loader2 } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { useDaemon } from '@/hooks/useDaemon';
import { ConfirmReversible } from '@/components/settings/ConfirmReversible';

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

export default function DaemonPage() {
  const { status, action } = useDaemon(5000);
  const [confirmAction, setConfirmAction] = useState<'stop' | 'restart' | null>(null);

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
      <Card className="border-zinc-800 bg-zinc-950/60">
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
          <div className="mt-4 flex gap-2">
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
