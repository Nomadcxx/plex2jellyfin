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
    <aside className="hidden w-60 bg-zinc-950 border-r border-zinc-800 h-screen p-4 md:flex md:flex-col">
      <div className="mb-8 flex items-center gap-3 px-1">
        <Image
          src="/plex2jellyfin_brand.png"
          alt=""
          width={32}
          height={32}
          className="rounded"
        />
        <h1 className="font-mono text-sm font-semibold tracking-tight text-zinc-100">
          plex2jellyfin
        </h1>
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
      <div className="mt-auto px-1 pb-1">
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
                ? 'flex min-w-16 flex-col items-center gap-1 rounded-md bg-zinc-900 px-2 py-1.5 text-xs font-medium text-terminal-cyan'
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
