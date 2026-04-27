'use client';

import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { useDaemon } from '@/hooks/useDaemon';
import { ConfirmReversible } from '@/components/settings/ConfirmReversible';

export default function DaemonPage() {
  const { status, action } = useDaemon(5000);
  const [confirmAction, setConfirmAction] = useState<'stop' | 'restart' | null>(null);

  if (!status) return <p>Loading…</p>;

  if (status.state === 'interrupted') {
    return (
      <div className="space-y-4">
        <h1 className="text-2xl font-semibold">Daemon</h1>
        <div className="rounded border border-destructive/50 bg-destructive/10 p-4">
          <p className="font-semibold">A previous destructive op was interrupted.</p>
          <pre className="mt-2 text-xs">{JSON.stringify(status.interrupted_op, null, 2)}</pre>
          <div className="mt-3 flex gap-2">
            <Button onClick={async () => {
              await fetch('/api/daemon/recover', {
                method: 'POST', headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ action: 'discard' }),
              });
              location.reload();
            }}>Discard</Button>
            <Button variant="outline" onClick={async () => {
              await fetch('/api/daemon/recover', {
                method: 'POST', headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ action: 'resume' }),
              });
              location.reload();
            }}>Resume</Button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">Daemon</h1>
      <div className="rounded border p-4">
        <p>State: <span className="font-mono">{status.state}</span></p>
        <p>Version: <span className="font-mono">{status.version}</span></p>
        <p>Uptime: <span className="font-mono">{status.uptime_s}s</span></p>
        <div className="mt-4 flex gap-2">
          <Button onClick={() => action('reload')}>Reload from disk</Button>
          <Button variant="outline" onClick={() => setConfirmAction('restart')}>Restart</Button>
          <Button variant="outline" onClick={() => setConfirmAction('stop')}>Stop</Button>
        </div>
      </div>

      <ConfirmReversible
        open={confirmAction !== null}
        title={`Confirm daemon ${confirmAction}`}
        onConfirm={async () => {
          if (confirmAction) await action(confirmAction);
          setConfirmAction(null);
        }}
        onCancel={() => setConfirmAction(null)}
      >
        Are you sure you want to {confirmAction} the daemon?
      </ConfirmReversible>
    </div>
  );
}
