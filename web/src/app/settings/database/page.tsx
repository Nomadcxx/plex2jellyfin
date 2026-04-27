'use client';

import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { ConfirmDestructive } from '@/components/settings/ConfirmDestructive';
import { ProgressCard } from '@/components/settings/ProgressCard';
import { useOpStream } from '@/hooks/useOpStream';

export default function DatabasePage() {
  const [opID, setOpID] = useState<string | null>(null);
  const [confirmReset, setConfirmReset] = useState(false);
  const { events } = useOpStream(opID);

  async function startRescan() {
    const r = await fetch('/api/v1/database/rescan', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ dry_run: false }),
    });
    const { op_id } = await r.json();
    setOpID(op_id);
  }

  async function startReset() {
    setConfirmReset(false);
    const r = await fetch('/api/v1/database/reset', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ confirm: 'media.db', preserve: ['audit_log'] }),
    });
    const { op_id } = await r.json();
    setOpID(op_id);
  }

  if (opID) {
    return (
      <div className="space-y-4">
        <h1 className="text-2xl font-semibold">Database</h1>
        <ProgressCard title="Operation in progress" events={events} />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold">Database</h1>

      <div className="rounded border p-4">
        <h2 className="font-semibold">Re-scan media</h2>
        <p className="text-sm text-muted-foreground">
          Re-walks all watch folders. Existing parse decisions and audit history are preserved.
        </p>
        <div className="mt-3">
          <Button onClick={startRescan}>Start re-scan</Button>
        </div>
      </div>

      <div className="rounded border border-destructive/30 p-4">
        <h2 className="font-semibold">Reset database</h2>
        <p className="text-sm text-muted-foreground">
          Permanently deletes indexed media, parse decisions, and operator overrides.
          Audit log is preserved.
        </p>
        <div className="mt-3">
          <Button variant="destructive" onClick={() => setConfirmReset(true)}>Reset database…</Button>
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
