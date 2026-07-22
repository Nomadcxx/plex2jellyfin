'use client';

import Link from 'next/link';
import Image from 'next/image';
import { usePathname } from 'next/navigation';
import {
  LayoutDashboard,
  Copy,
  Download,
  Activity,
  FolderSync,
  Settings,
  Calendar,
  Route,
  ListChecks,
} from 'lucide-react';
import { LogoutButton } from '@/components/auth/LogoutButton';

const navigation = [
  { name: 'Dashboard', href: '/', icon: LayoutDashboard },
  { name: 'Duplicates', href: '/duplicates', icon: Copy },
  { name: 'Queue', href: '/queue', icon: Download },
  { name: 'Activity', href: '/activity', icon: Activity },
  { name: 'Jellyfin', href: '/jellyfin', icon: ListChecks },
  { name: 'Trace', href: '/trace', icon: Route },
  { name: 'Consolidation', href: '/consolidation', icon: FolderSync },
  { name: 'Scheduler', href: '/scheduler', icon: Calendar },
  { name: 'Settings', href: '/settings', icon: Settings },
];

export function Sidebar() {
  const pathname = usePathname();

  const isActive = (href: string) =>
    href === '/' ? pathname === '/' : pathname === href || pathname.startsWith(href + '/');

  return (
    <aside className="hidden w-60 bg-zinc-950 border-r border-zinc-800 h-screen p-4 md:flex md:flex-col">
      <div className="mb-8 px-1">
        <Image
          src="/p2j-mark.png"
          alt="plex2jellyfin"
          width={140}
          height={50}
          priority
          className="h-auto w-[120px]"
        />
      </div>
      <nav className="space-y-1">
        {navigation.map((item) => (
          <Link
            key={item.name}
            href={item.href}
            className={
              isActive(item.href)
                ? 'flex items-center gap-3 border-l-2 border-terminal-cyan bg-zinc-900 px-3 py-2 font-mono text-[13px] text-terminal-cyan'
                : 'flex items-center gap-3 border-l-2 border-transparent px-3 py-2 font-mono text-[13px] text-zinc-400 hover:bg-zinc-900 hover:text-zinc-100'
            }
            aria-current={isActive(item.href) ? 'page' : undefined}
          >
            <item.icon className="h-4 w-4 shrink-0" />
            {item.name}
          </Link>
        ))}
      </nav>
      <div className="mt-auto space-y-3 px-1 pb-1">
        <LogoutButton className="flex w-full items-center gap-3 border-l-2 border-transparent px-3 py-2 font-mono text-[13px] text-zinc-400 hover:bg-zinc-900 hover:text-zinc-100 disabled:opacity-50" />
        <p className="term-eyebrow">beta</p>
      </div>
    </aside>
  );
}

export function MobileNav() {
  const pathname = usePathname();

  const isActive = (href: string) =>
    href === '/' ? pathname === '/' : pathname === href || pathname.startsWith(href + '/');

  return (
    <nav
      className="fixed inset-x-0 bottom-0 z-40 border-t border-zinc-800 bg-zinc-950 px-2 py-2 md:hidden"
      aria-label="Primary"
    >
      <div className="flex items-center gap-1 overflow-x-auto">
        {navigation.map((item) => (
          <Link
            key={item.name}
            href={item.href}
            className={
              isActive(item.href)
                ? 'flex min-w-16 flex-col items-center gap-1 rounded-md bg-zinc-900 px-2 py-1.5 text-xs font-medium text-terminal-cyan'
                : 'flex min-w-16 flex-col items-center gap-1 rounded-md px-2 py-1.5 text-xs text-zinc-400 hover:bg-zinc-900 hover:text-zinc-100'
            }
            aria-current={isActive(item.href) ? 'page' : undefined}
          >
            <item.icon className="h-4 w-4 shrink-0" />
            <span>{item.name}</span>
          </Link>
        ))}
        <LogoutButton
          iconOnly
          className="flex min-w-16 flex-col items-center gap-1 rounded-md px-2 py-1.5 text-xs text-zinc-400 hover:bg-zinc-900 hover:text-zinc-100 disabled:opacity-50"
        />
      </div>
    </nav>
  );
}
