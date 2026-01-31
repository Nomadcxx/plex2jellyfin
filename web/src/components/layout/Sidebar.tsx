'use client';

import Link from 'next/link';
import { LayoutDashboard, Copy, Download, Activity, FolderSync } from 'lucide-react';

const navigation = [
  { name: 'Dashboard', href: '/', icon: LayoutDashboard },
  { name: 'Duplicates', href: '/duplicates', icon: Copy },
  { name: 'Queue', href: '/queue', icon: Download },
  { name: 'Activity', href: '/activity', icon: Activity },
  { name: 'Consolidation', href: '/consolidation', icon: FolderSync },
];

export function Sidebar() {
  return (
    <aside className="w-64 bg-zinc-900 border-r border-zinc-800 h-screen p-4">
      <div className="mb-8">
        <h1 className="text-xl font-bold">JellyWatch</h1>
      </div>
      <nav className="space-y-2">
        {navigation.map((item) => (
          <Link
            key={item.name}
            href={item.href}
            className="flex items-center gap-3 px-3 py-2 rounded-lg text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100"
          >
            <item.icon className="h-4 w-4" />
            {item.name}
          </Link>
        ))}
      </nav>
    </aside>
  );
}
