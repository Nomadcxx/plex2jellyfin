'use client';

import { useEffect, useState } from 'react';

export type DaemonStatus = {
  state: 'running' | 'stopped' | 'interrupted';
  version?: string;
  uptime_s?: number;
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
        const r = await fetch('/api/daemon/status');
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
    await fetch(`/api/daemon/${name}`, { method: 'POST' });
  }

  return { status, action };
}
