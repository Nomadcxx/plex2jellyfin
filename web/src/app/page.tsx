'use client';

import Image from 'next/image';
import Link from 'next/link';
import { AppShell } from '@/components/layout/AppShell';
import { useDashboard, useJellyfinIdentification } from '@/hooks/useDashboard';
import { useDuplicates } from '@/hooks/useDashboard';
import { useJellystatOverview, mostViewedName, mostViewedPlays, JellystatRow } from '@/hooks/useJellystat';
import { useDaemon } from '@/hooks/useDaemon';
import { RecentlyAdded } from '@/components/dashboard/RecentlyAdded';
import { formatBytes } from '@/lib/utils';
import { Database, HardDrive, Copy, FolderTree, Film, Tv, ListVideo, AlertTriangle, CheckCircle2, HelpCircle, BarChart3, Activity, Server } from 'lucide-react';
import { Alert, AlertDescription } from '@/components/ui/alert';

export default function DashboardPage() {
  const { data, isLoading, isError, error } = useDashboard();
  const { data: dupData } = useDuplicates();
  const liveDuplicateGroups = dupData?.groups?.length ?? 0;
  const dbDuplicateGroups = data?.libraryStats?.duplicateGroups ?? 0;
  const duplicateGroups = dbDuplicateGroups || liveDuplicateGroups;
  const { data: jfId, isLoading: jfLoading, isError: jfError } = useJellyfinIdentification();
  const { status: daemonStatus } = useDaemon();

  return (
    <AppShell>
      <div className="space-y-6">
        <div className="flex items-center gap-4">
          <Image
            src="/p2j-mark.png"
            alt="Plex2Jellyfin"
            width={160}
            height={58}
            priority
            className="h-auto w-[140px] md:hidden"
          />
          <h1 className="text-3xl font-bold">Dashboard</h1>
        </div>

        <SystemHealthBanner
          daemonStatus={daemonStatus}
          totalFiles={data?.libraryStats?.totalFiles ?? 0}
          totalSize={data?.libraryStats?.totalSize ?? 0}
        />

        {isError && (
          <Alert variant="destructive">
            <AlertTriangle className="h-4 w-4" />
            <AlertDescription>
              Failed to load dashboard: {(error as Error)?.message ?? 'Unknown error'}
            </AlertDescription>
          </Alert>
        )}

        {isLoading ? (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
            {[1, 2, 3, 4].map((i) => (
              <div key={i} className="h-32 vision-card animate-pulse" />
            ))}
          </div>
        ) : (
          <>
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
              <StatCard
                title="Total Files"
                value={data?.libraryStats?.totalFiles?.toLocaleString() || '0'}
                icon={Database}
              />
              <StatCard
                title="Library Size"
                value={formatBytes(data?.libraryStats?.totalSize || 0)}
                icon={HardDrive}
              />
              <Link href="/duplicates" className="block rounded-xl transition-opacity hover:opacity-80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-zinc-950">
                <StatCard
                  title="Duplicates"
                  value={duplicateGroups.toLocaleString()}
                  icon={Copy}
                />
              </Link>
              <StatCard
                title="Scattered Series"
                value={(data?.libraryStats?.scatteredSeries ?? 0).toLocaleString()}
                icon={FolderTree}
              />
            </div>

            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <StatCard
                title="Movies"
                value={(data?.libraryStats?.movieCount ?? 0).toLocaleString()}
                icon={Film}
              />
              <StatCard
                title="Series"
                value={(data?.libraryStats?.seriesCount ?? 0).toLocaleString()}
                icon={Tv}
              />
              <StatCard
                title="Episodes"
                value={(data?.libraryStats?.episodeCount ?? 0).toLocaleString()}
                icon={ListVideo}
              />
            </div>
          </>
        )}

        <div className="mt-8">
          <h2 className="text-xl font-semibold mb-4">Jellyfin Identification</h2>
          <p className="mb-4 text-sm text-zinc-500">
            Tracks files plex2jellyfin organized and whether Jellyfin resolved their metadata
            (via the companion plugin). Not Jellystat playback stats.
          </p>
          {jfId ? (
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <Link href="/jellyfin?status=identified" className="block rounded-xl transition-opacity hover:opacity-80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-zinc-950">
                <StatCard
                  title="Resolved"
                  value={`${jfId.resolved.toLocaleString()} / ${jfId.total.toLocaleString()}`}
                  icon={CheckCircle2}
                />
              </Link>
              <Link href="/jellyfin?status=identified" className="block rounded-xl transition-opacity hover:opacity-80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-zinc-950">
                <StatCard
                  title="Identified"
                  value={`${jfId.identified.toLocaleString()} (${(jfId.identified_pct_x10 / 10).toFixed(1)}%)`}
                  icon={CheckCircle2}
                />
              </Link>
              <Link href="/jellyfin?status=unidentified" className="block rounded-xl transition-opacity hover:opacity-80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-zinc-950">
                <StatCard
                  title="Unidentified"
                  value={jfId.unidentified.toLocaleString()}
                  icon={HelpCircle}
                />
              </Link>
              <Link href="/jellyfin?status=failed" className="block rounded-xl transition-opacity hover:opacity-80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-zinc-950">
                <StatCard
                  title="Failed"
                  value={jfId.failed_auto_label.toLocaleString()}
                  icon={AlertTriangle}
                />
              </Link>
            </div>
          ) : jfLoading ? (
            <p className="text-sm text-zinc-500">Loading identification stats…</p>
          ) : jfError ? (
            <p className="text-sm text-amber-400">Could not load identification stats.</p>
          ) : (
            <p className="text-sm text-zinc-500">No organized files yet — stats appear after imports.</p>
          )}
        </div>

        <RecentlyAdded />

        <JellystatSection />

        <div className="mt-8">
          <h2 className="text-xl font-semibold mb-4">Media Managers</h2>
          <div className="space-y-3">
            {data?.mediaManagers?.map((manager: any) => (
              <div key={manager.id} className="vision-card-secondary flex items-center justify-between p-4">
                <div>
                  <p className="font-medium">{manager.name}</p>
                  <p className="text-sm text-zinc-400 capitalize">{manager.type}</p>
                </div>
                <span className={`text-sm ${manager.online ? 'text-terminal-green' : 'text-terminal-red'}`}>
                  {manager.online ? '● Online' : '● Offline'}
                </span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </AppShell>
  );
}

function SystemHealthBanner({
  daemonStatus,
  totalFiles,
  totalSize,
}: {
  daemonStatus: { state: string; uptime_seconds?: number; version?: string } | null;
  totalFiles: number;
  totalSize: number;
}) {
  const isRunning = daemonStatus?.state === 'running';
  const statusColor = isRunning ? 'text-terminal-green' : 'text-terminal-red';
  const statusDot = isRunning ? 'bg-terminal-green' : 'bg-terminal-red';

  return (
    <div className="vision-banner flex flex-wrap items-center gap-x-6 gap-y-2 p-4">
      <div className="flex items-center gap-2">
        <span className={`h-2 w-2 rounded-full ${statusDot} ${isRunning ? 'animate-pulse' : ''}`} />
        <span className={`font-mono text-sm ${statusColor}`}>
          {isRunning ? 'daemon running' : 'daemon stopped'}
        </span>
      </div>
      {daemonStatus?.uptime_seconds != null && isRunning && (
        <span className="font-mono text-sm text-zinc-500">
          uptime {formatUptime(daemonStatus.uptime_seconds)}
        </span>
      )}
      <span className="font-mono text-sm text-zinc-400">
        {totalFiles.toLocaleString()} files · {formatBytes(totalSize)}
      </span>
      {daemonStatus?.version && (
        <span className="font-mono text-xs text-zinc-500">v{daemonStatus.version}</span>
      )}
    </div>
  );
}

function formatUptime(seconds: number): string {
  if (seconds < 60) return `${Math.floor(seconds)}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`;
  return `${Math.floor(seconds / 86400)}d ${Math.floor((seconds % 86400) / 3600)}h`;
}

function MostViewedList({ title, rows }: { title: string; rows?: JellystatRow[] }) {
  return (
    <div className="vision-card-secondary p-6">
      <p className="text-sm text-zinc-400 mb-3">{title}</p>
      {rows && rows.length > 0 ? (
        <ol className="space-y-2">
          {rows.slice(0, 5).map((row, i) => {
            const plays = mostViewedPlays(row);
            return (
              <li key={i} className="flex items-baseline justify-between gap-3 text-sm">
                <span className="text-zinc-200 truncate">
                  <span className="text-zinc-500 mr-2">{i + 1}.</span>
                  {mostViewedName(row)}
                </span>
                {plays != null && (
                  <span className="text-zinc-500 whitespace-nowrap tabular-nums">{plays} plays</span>
                )}
              </li>
            );
          })}
        </ol>
      ) : (
        <p className="text-sm text-zinc-400">No plays recorded in the last 30 days.</p>
      )}
    </div>
  );
}

// Watch statistics from Jellystat. Renders nothing unless the integration
// is enabled in Settings → Jellystat.
function JellystatSection() {
  const { data } = useJellystatOverview();
  if (!data?.enabled) return null;

  return (
    <div className="mt-8">
      <h2 className="text-xl font-semibold mb-4 flex items-center gap-2">
        <BarChart3 className="h-5 w-5 text-zinc-500" />
        Watch Statistics
        <span className="text-xs font-normal text-zinc-500">via Jellystat, last 30 days</span>
      </h2>
      {data.error && (
        <Alert variant="destructive" className="mb-4">
          <AlertTriangle className="h-4 w-4" />
          <AlertDescription>Jellystat error: {data.error}</AlertDescription>
        </Alert>
      )}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <MostViewedList title="Most watched movies" rows={data.most_viewed_movies} />
        <MostViewedList title="Most watched series" rows={data.most_viewed_series} />
      </div>
    </div>
  );
}

function StatCard({ title, value, icon: Icon }: { title: string; value: string | number; icon: any }) {
  return (
    <div className="vision-card p-6">
      <div className="flex items-center justify-between">
        <div>
          <p className="text-sm text-zinc-400">{title}</p>
          <p className="text-2xl font-bold mt-1 tabular-nums">{value}</p>
        </div>
        <div className="vision-icon-circle">
          <Icon className="h-5 w-5 text-terminal-amber" />
        </div>
      </div>
    </div>
  );
}
