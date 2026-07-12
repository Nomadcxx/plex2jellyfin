'use client';

import Link from 'next/link';
import { Settings } from 'lucide-react';
import { useDaemon } from '@/hooks/useDaemon';
import { useSettingsSection } from '@/hooks/useSettings';

function StatusRow({
  label,
  value,
  href,
  tone = 'neutral',
}: {
  label: string;
  value: string;
  href?: string;
  tone?: 'ok' | 'warn' | 'bad' | 'neutral';
}) {
  const toneClass =
    tone === 'ok'
      ? 'text-green-400'
      : tone === 'warn'
        ? 'text-amber-300'
        : tone === 'bad'
          ? 'text-red-400'
          : 'text-zinc-200';

  const body = (
    <div className="flex items-baseline justify-between gap-4 border-b border-zinc-900 py-3">
      <dt className="font-mono text-xs uppercase tracking-[0.14em] text-zinc-500">{label}</dt>
      <dd className={`text-sm ${toneClass}`}>{value}</dd>
    </div>
  );

  if (!href) return body;
  return (
    <Link href={href} className="block hover:bg-zinc-900/40">
      {body}
    </Link>
  );
}

function integrationLabel(enabled: boolean | undefined, loading: boolean) {
  if (loading) return { value: '…', tone: 'neutral' as const };
  if (enabled) return { value: 'Enabled', tone: 'ok' as const };
  return { value: 'Off', tone: 'neutral' as const };
}

export default function SettingsPage() {
  const { status } = useDaemon(5000);
  const sonarr = useSettingsSection('sonarr');
  const radarr = useSettingsSection('radarr');
  const jellyfin = useSettingsSection('jellyfin');
  const jellystat = useSettingsSection('jellystat');
  const ai = useSettingsSection('ai');

  const daemonTone =
    status?.state === 'running' ? 'ok' : status?.state === 'interrupted' ? 'warn' : 'bad';

  return (
    <div className="space-y-8">
      <div>
        <h1 className="flex items-center gap-3 text-2xl font-semibold tracking-tight">
          <Settings className="h-6 w-6 text-zinc-400" />
          Overview
        </h1>
        <p className="mt-2 max-w-xl text-sm text-zinc-400">
          Runtime snapshot for this install. Use the rail to open a section — nothing here
          duplicates those destinations.
        </p>
      </div>

      <section>
        <h2 className="font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-600">Runtime</h2>
        <dl className="mt-1">
          <StatusRow
            label="Daemon"
            value={status?.state ?? 'unknown'}
            href="/settings/daemon"
            tone={daemonTone}
          />
          <StatusRow
            label="Uptime"
            value={
              status?.uptime_seconds != null
                ? `${Math.floor(status.uptime_seconds / 60)}m`
                : '—'
            }
          />
          <StatusRow label="Version" value={status?.version ?? '—'} />
        </dl>
      </section>

      <section>
        <h2 className="font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-600">
          Integrations
        </h2>
        <dl className="mt-1">
          <StatusRow
            label="Sonarr"
            href="/settings/sonarr"
            {...integrationLabel((sonarr.data as { enabled?: boolean } | undefined)?.enabled, sonarr.isLoading)}
          />
          <StatusRow
            label="Radarr"
            href="/settings/radarr"
            {...integrationLabel((radarr.data as { enabled?: boolean } | undefined)?.enabled, radarr.isLoading)}
          />
          <StatusRow
            label="Jellyfin"
            href="/settings/jellyfin"
            {...integrationLabel((jellyfin.data as { enabled?: boolean } | undefined)?.enabled, jellyfin.isLoading)}
          />
          <StatusRow
            label="Jellystat"
            href="/settings/jellystat"
            {...integrationLabel((jellystat.data as { enabled?: boolean } | undefined)?.enabled, jellystat.isLoading)}
          />
          <StatusRow
            label="AI"
            href="/settings/ai"
            {...integrationLabel((ai.data as { enabled?: boolean } | undefined)?.enabled, ai.isLoading)}
          />
        </dl>
      </section>
    </div>
  );
}
