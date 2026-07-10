'use client';

import Link from 'next/link';
import Image from 'next/image';
import { usePathname } from 'next/navigation';
import { LayoutDashboard, Copy, Download, Activity, FolderSync, Settings, Calendar, Route } from 'lucide-react';

const navigation = [
  { name: 'Dashboard', href: '/', icon: LayoutDashboard },
  { name: 'Duplicates', href: '/duplicates', icon: Copy },
  { name: 'Queue', href: '/queue', icon: Download },
  { name: 'Activity', href: '/activity', icon: Activity },
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
    <aside className="hidden w-64 bg-zinc-900 border-r border-zinc-800 h-screen p-4 md:block">
      <div className="mb-8 flex items-center gap-3">
        <Image
          src="/jellywooch.png"
          alt="Plex2Jellyfin"
          width={36}
          height={36}
          className="rounded"
        />
        <h1 className="text-xl font-bold">Plex2Jellyfin</h1>
      </div>
      <nav className="space-y-2">
        {navigation.map((item) => (
          <Link
            key={item.name}
            href={item.href}
            className={
              isActive(item.href)
                ? 'flex items-center gap-3 px-3 py-2 rounded-lg bg-fuchsia-500/15 text-fuchsia-300 font-medium'
                : 'flex items-center gap-3 px-3 py-2 rounded-lg text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100'
            }
            aria-current={isActive(item.href) ? 'page' : undefined}
          >
            <item.icon className="h-4 w-4 shrink-0" />
            {item.name}
          </Link>
        ))}
      </nav>
    </aside>
  );
}

export function MobileNav() {
  const pathname = usePathname();

  const isActive = (href: string) =>
    href === '/' ? pathname === '/' : pathname === href || pathname.startsWith(href + '/');

  return (
    <nav
      className="fixed inset-x-0 bottom-0 z-40 border-t border-zinc-800 bg-zinc-950/95 px-2 py-2 backdrop-blur md:hidden"
      aria-label="Primary"
    >
      <div className="flex items-center gap-1 overflow-x-auto">
        {navigation.map((item) => (
          <Link
            key={item.name}
            href={item.href}
            className={
              isActive(item.href)
                ? 'flex min-w-16 flex-col items-center gap-1 rounded-md bg-fuchsia-500/15 px-2 py-1.5 text-xs font-medium text-fuchsia-300'
                : 'flex min-w-16 flex-col items-center gap-1 rounded-md px-2 py-1.5 text-xs text-zinc-400 hover:bg-zinc-900 hover:text-zinc-100'
            }
            aria-current={isActive(item.href) ? 'page' : undefined}
          >
            <item.icon className="h-4 w-4 shrink-0" />
            <span>{item.name}</span>
          </Link>
        ))}
      </div>
    </nav>
  );
}
