'use client';

import { useState } from 'react';
import { Loader2, RefreshCw, ShieldCheck } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import {
  getJellyfinPluginStatus,
  JellyfinPluginStatus,
  verifyJellyfinPlugin,
  VerifyJellyfinPluginResult,
} from '@/lib/api/client';

export function PluginStatusPanel() {
  const [status, setStatus] = useState<JellyfinPluginStatus | null>(null);
  const [verify, setVerify] = useState<VerifyJellyfinPluginResult | null>(null);
  const [busy, setBusy] = useState<'status' | 'verify' | null>(null);

  const refresh = async () => {
    setBusy('status');
    setVerify(null);
    try {
      const next = await getJellyfinPluginStatus();
      setStatus(next);
      if (next.message && next.healthy === false && next.installed === false) {
        toast.message(next.message);
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Plugin status failed');
    } finally {
      setBusy(null);
    }
  };

  const runVerify = async () => {
    setBusy('verify');
    try {
      const result = await verifyJellyfinPlugin();
      setVerify(result);
      if (result.ok) {
        toast.success('Plugin feedback loop verified');
      } else {
        toast.error(result.error || 'Verification failed');
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Verification failed');
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="space-y-3 border border-zinc-800 bg-zinc-950/40 p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="font-mono text-sm font-semibold text-zinc-200">Companion plugin</h2>
          <p className="mt-1 text-sm text-zinc-500">
            Status of the Plex2Jellyfin Jellyfin plugin and webhook round-trip.
          </p>
        </div>
        <div className="flex gap-2">
          <Button type="button" variant="outline" size="sm" disabled={busy !== null} onClick={refresh}>
            {busy === 'status' ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
            Status
          </Button>
          <Button type="button" variant="outline" size="sm" disabled={busy !== null} onClick={runVerify}>
            {busy === 'verify' ? <Loader2 className="h-4 w-4 animate-spin" /> : <ShieldCheck className="h-4 w-4" />}
            Verify
          </Button>
        </div>
      </div>

      {status && (
        <div className="space-y-1 text-sm">
          <p>
            <span className="text-zinc-400">Installed:</span>{' '}
            <span className="font-mono">{status.installed ? status.version || 'yes' : 'no'}</span>
            {' · '}
            <span className="text-zinc-400">Healthy:</span>{' '}
            <span className={status.healthy ? 'text-emerald-300' : 'text-amber-200'}>
              {status.healthy ? 'yes' : 'no'}
            </span>
          </p>
          {status.message && <p className="text-zinc-500">{status.message}</p>}
        </div>
      )}

      {verify && (
        <p className={`text-sm ${verify.ok ? 'text-emerald-300' : 'text-rose-300'}`}>
          {verify.ok
            ? 'Verified — signed test event reached the daemon.'
            : verify.error || 'Verification failed'}
        </p>
      )}
    </div>
  );
}
