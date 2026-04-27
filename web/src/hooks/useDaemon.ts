'use client';

import { useEffect, useState } from 'react';

export type DaemonStatus = {
  state: 'running' | 'stopped' | 'interrupted';
  version?: string;
  uptime_seconds?: number;
  current_op?: { id: string; cmd: string };
  config_hash?: string;
  interrupted_op?: any;
};

export function useDaemon(intervalMs = 5000) {
  const [status, setStatus] = useState<DaemonStatus | null>(null);

  useEffect(() => {
    let cancelled = false;
    async function tick() {
      try {
        const r = await fetch('/api/v1/daemon/status');
        if (!cancelled && r.ok) setStatus(await r.json());
      } catch {
        if (!cancelled) setStatus({ state: 'stopped' });
      }
    }
    tick();
    const id = setInterval(tick, intervalMs);
    return () => { cancelled = true; clearInterval(id); };
  }, [intervalMs]);

  async function action(name: 'start' | 'stop' | 'restart' | 'reload') {
    const r = await fetch(`/api/v1/daemon/${name}`, { method: 'POST' });
    if (!r.ok) throw new Error(`Daemon ${name} failed (${r.status})`);
  }

  return { status, action };
}
