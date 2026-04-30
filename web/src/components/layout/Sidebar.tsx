'use client';

import Link from 'next/link';
import Image from 'next/image';
import { usePathname } from 'next/navigation';
import { LayoutDashboard, Copy, Download, Activity, FolderSync, Settings, Calendar } from 'lucide-react';

const navigation = [
  { name: 'Dashboard', href: '/', icon: LayoutDashboard },
  { name: 'Duplicates', href: '/duplicates', icon: Copy },
  { name: 'Queue', href: '/queue', icon: Download },
  { name: 'Activity', href: '/activity', icon: Activity },
  { name: 'Consolidation', href: '/consolidation', icon: FolderSync },
  { name: 'Scheduler', href: '/scheduler', icon: Calendar },
  { name: 'Settings', href: '/settings', icon: Settings },
];

export function Sidebar() {
  const pathname = usePathname();

  const isActive = (href: string) =>
    href === '/' ? pathname === '/' : pathname === href || pathname.startsWith(href + '/');

  return (
    <aside className="w-64 bg-zinc-900 border-r border-zinc-800 h-screen p-4">
      <div className="mb-8 flex items-center gap-3">
        <Image
          src="/jellywooch.png"
          alt="JellyWatch"
          width={36}
          height={36}
          className="rounded"
        />
        <h1 className="text-xl font-bold">JellyWatch</h1>
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
