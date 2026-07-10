'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { Bot, Database, FolderOpen, HardDrive, ListChecks, LockKeyhole, Radio, Search, Server, Settings, ShieldCheck, SlidersHorizontal, Wrench } from 'lucide-react';
import { AppShell } from '@/components/layout/AppShell';
import { useDaemon } from '@/hooks/useDaemon';

const nav = [
  { href: '/settings', label: 'Overview', icon: Settings },
  { href: '/settings/paths', label: 'Paths', icon: FolderOpen },
  { href: '/settings/libraries', label: 'Libraries', icon: HardDrive },
  { href: '/settings/sonarr', label: 'Sonarr', icon: Radio },
  { href: '/settings/radarr', label: 'Radarr', icon: Radio },
  { href: '/settings/jellyfin', label: 'Jellyfin', icon: ListChecks },
  { href: '/settings/tmdb', label: 'TMDB', icon: Search },
  { href: '/settings/ai', label: 'AI', icon: Bot },
  { href: '/settings/options', label: 'Options', icon: SlidersHorizontal },
  { href: '/settings/logging', label: 'Logging', icon: Wrench },
  { href: '/settings/permissions', label: 'Permissions', icon: LockKeyhole },
  { href: '/settings/daemon', label: 'Daemon', icon: Server },
  { href: '/settings/indexing', label: 'Indexing', icon: Search },
  { href: '/settings/database', label: 'Database', icon: Database },
  { href: '/settings/security', label: 'Security', icon: ShieldCheck },
];

function daemonDotColor(state?: string) {
  switch (state) {
    case 'running': return 'bg-green-500';
    case 'interrupted': return 'bg-yellow-500';
    case 'stopped': return 'bg-red-500';
    default: return 'bg-zinc-500';
  }
}

export default function SettingsLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const { status } = useDaemon(5000);

  return (
    <AppShell>
      <div className="mx-auto grid max-w-7xl gap-6 lg:grid-cols-[220px_minmax(0,1fr)]">
        <nav className="space-y-1 lg:sticky lg:top-6 lg:self-start">
          {nav.map((item) => {
            const active = pathname === item.href;
            const Icon = item.icon;
            const isDaemon = item.href === '/settings/daemon';
            return (
              <Link
                key={item.href}
                href={item.href}
                className={`flex items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors ${
                  active ? 'bg-zinc-800 text-zinc-100' : 'text-zinc-400 hover:bg-zinc-900 hover:text-zinc-200'
                }`}
              >
                <Icon className="h-4 w-4" />
                {item.label}
                {isDaemon && (
                  <span
                    aria-label={`daemon ${status?.state ?? 'unknown'}`}
                    className={`ml-auto inline-block h-2 w-2 rounded-full ${daemonDotColor(status?.state)}`}
                  />
                )}
              </Link>
            );
          })}
        </nav>
        <section className="min-w-0 space-y-6 pb-12">{children}</section>
      </div>
    </AppShell>
  );
}

