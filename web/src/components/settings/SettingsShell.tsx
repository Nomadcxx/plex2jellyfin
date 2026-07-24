'use client';

import type { ComponentType, ReactNode } from 'react';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import {
  ArrowLeft,
  BarChart3,
  Bot,
  Database,
  FolderOpen,
  HardDrive,
  ListChecks,
  LockKeyhole,
  Radio,
  Search,
  Server,
  Settings,
  ShieldCheck,
  SlidersHorizontal,
  Wrench,
} from 'lucide-react';
import { JobsTray } from '@/components/layout/JobsTray';
import { TermPrompt } from '@/components/layout/TermPrompt';
import { ScanButton } from '@/components/scan/ScanButton';
import { useDaemon } from '@/hooks/useDaemon';
import { LogoutButton } from '@/components/auth/LogoutButton';

type NavItem = {
  href: string;
  label: string;
  icon: ComponentType<{ className?: string }>;
  daemonDot?: boolean;
};

type NavGroup = {
  label: string;
  items: NavItem[];
};

const overviewItem: NavItem = { href: '/settings', label: 'Overview', icon: Settings };

const groups: NavGroup[] = [
  {
    label: 'Media',
    items: [
      { href: '/settings/paths', label: 'Paths', icon: FolderOpen },
      { href: '/settings/libraries', label: 'Libraries', icon: HardDrive },
    ],
  },
  {
    label: 'Integrations',
    items: [
      { href: '/settings/sonarr', label: 'Sonarr', icon: Radio },
      { href: '/settings/radarr', label: 'Radarr', icon: Radio },
      { href: '/settings/jellyfin', label: 'Jellyfin', icon: ListChecks },
      { href: '/settings/jellystat', label: 'Jellystat', icon: BarChart3 },
      { href: '/settings/tmdb', label: 'TMDB', icon: Search },
      { href: '/settings/ai', label: 'AI', icon: Bot },
    ],
  },
  {
    label: 'Runtime',
    items: [
      { href: '/settings/options', label: 'Options', icon: SlidersHorizontal },
      { href: '/settings/logging', label: 'Logging', icon: Wrench },
      { href: '/settings/permissions', label: 'Permissions', icon: LockKeyhole },
      { href: '/settings/daemon', label: 'Daemon', icon: Server, daemonDot: true },
      { href: '/settings/indexing', label: 'Indexing', icon: Search },
      { href: '/settings/database', label: 'Database', icon: Database },
    ],
  },
  {
    label: 'Account',
    items: [{ href: '/settings/security', label: 'Security', icon: ShieldCheck }],
  },
];

function daemonDotColor(state?: string) {
  switch (state) {
    case 'running':
      return 'bg-green-500';
    case 'interrupted':
      return 'bg-yellow-500';
    case 'stopped':
      return 'bg-terminal-red';
    default:
      return 'bg-zinc-500';
  }
}

function NavLink({
  item,
  active,
  daemonState,
}: {
  item: NavItem;
  active: boolean;
  daemonState?: string;
}) {
  const Icon = item.icon;
  return (
    <Link
      href={item.href}
      className={`flex items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors ${
        active ? 'vision-nav-active text-terminal-amber' : 'text-zinc-400 hover:bg-zinc-900 hover:text-zinc-200'
      }`}
      aria-current={active ? 'page' : undefined}
    >
      <Icon className="h-4 w-4 shrink-0" />
      <span className="truncate">{item.label}</span>
      {item.daemonDot && (
        <span
          aria-label={`daemon ${daemonState ?? 'unknown'}`}
          className={`ml-auto inline-block h-2 w-2 rounded-full ${daemonDotColor(daemonState)}`}
        />
      )}
    </Link>
  );
}

function SettingsRail({ daemonState }: { daemonState?: string }) {
  const pathname = usePathname();
  const route = pathname.length > 1 ? pathname.replace(/\/+$/, '') : pathname;

  return (
    <aside className="hidden w-56 shrink-0 flex-col border-r border-zinc-800 bg-zinc-950 p-4 md:flex">
      <Link
        href="/"
        className="mb-6 inline-flex items-center gap-2 px-1 font-mono text-xs text-zinc-400 hover:text-amber-300"
      >
        <ArrowLeft className="h-3.5 w-3.5" />
        Dashboard
      </Link>
      <nav className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto">
        <div className="space-y-1">
          <NavLink item={overviewItem} active={route === '/settings'} daemonState={daemonState} />
        </div>
        {groups.map((group) => (
          <div key={group.label} className="space-y-1">
            <p className="px-3 font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-600">
              {group.label}
            </p>
            {group.items.map((item) => (
              <NavLink
                key={item.href}
                item={item}
                active={route === item.href}
                daemonState={daemonState}
              />
            ))}
          </div>
        ))}
      </nav>
      <LogoutButton className="mt-4 flex w-full items-center gap-2 rounded-md px-3 py-2 font-mono text-sm text-zinc-400 hover:bg-zinc-900 hover:text-zinc-100 disabled:opacity-50" />
    </aside>
  );
}

function SettingsMobileNav({ daemonState }: { daemonState?: string }) {
  const pathname = usePathname();
  const route = pathname.length > 1 ? pathname.replace(/\/+$/, '') : pathname;
  const flat = [overviewItem, ...groups.flatMap((g) => g.items)];

  return (
    <div className="border-b border-zinc-800 md:hidden">
      <div className="flex items-center gap-2 overflow-x-auto px-3 py-2">
        <Link
          href="/"
          className="inline-flex shrink-0 items-center gap-1 rounded-md border border-zinc-800 px-2 py-1.5 font-mono text-xs text-zinc-300"
        >
          <ArrowLeft className="h-3 w-3" />
          Home
        </Link>
        {flat.map((item) => {
          const Icon = item.icon;
          const active = route === item.href;
          return (
            <Link
              key={item.href}
              href={item.href}
              className={`inline-flex shrink-0 items-center gap-1.5 rounded-md px-2 py-1.5 text-xs ${
                active ? 'vision-nav-active text-terminal-amber' : 'text-zinc-400'
              }`}
              aria-current={active ? 'page' : undefined}
            >
              <Icon className="h-3.5 w-3.5" />
              {item.label}
              {item.daemonDot && (
                <span className={`h-1.5 w-1.5 rounded-full ${daemonDotColor(daemonState)}`} />
              )}
            </Link>
          );
        })}
      </div>
    </div>
  );
}

export function SettingsShell({ children }: { children: ReactNode }) {
  const { status } = useDaemon(5000);

  return (
    <div className="flex min-h-screen bg-zinc-950 text-zinc-100">
      <SettingsRail daemonState={status?.state} />
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        <header className="flex items-center justify-between gap-3 border-b border-zinc-800 bg-zinc-950/80 px-4 py-3 backdrop-blur md:px-6">
          <div className="min-w-0">
            <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-600">Settings</p>
            <TermPrompt />
          </div>
          <div className="flex items-center gap-3">
            <JobsTray />
            <ScanButton />
          </div>
        </header>
        <SettingsMobileNav daemonState={status?.state} />
        <main className="flex-1 overflow-auto p-4 md:p-6">
          <div className="mx-auto max-w-3xl pb-12">{children}</div>
        </main>
      </div>
    </div>
  );
}
