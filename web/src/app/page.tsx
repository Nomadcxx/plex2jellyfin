'use client';

import { AppShell } from '@/components/layout/AppShell';
import { useDashboard } from '@/hooks/useDashboard';
import { formatBytes } from '@/lib/utils';
import { Database, HardDrive, Copy, FolderTree } from 'lucide-react';

export default function DashboardPage() {
  const { data, isLoading } = useDashboard();

  return (
    <AppShell>
      <div className="space-y-6">
        <h1 className="text-3xl font-bold">Dashboard</h1>
        
        {isLoading ? (
          <div className="grid grid-cols-4 gap-4">
            {[1, 2, 3, 4].map((i) => (
              <div key={i} className="h-32 bg-zinc-900 rounded-lg animate-pulse" />
            ))}
          </div>
        ) : (
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
            <StatCard
              title="Duplicates"
              value={data?.libraryStats?.duplicateGroups || 0}
              icon={Copy}
            />
            <StatCard
              title="Scattered Series"
              value={data?.libraryStats?.scatteredSeries || 0}
              icon={FolderTree}
            />
          </div>
        )}

        <div className="mt-8">
          <h2 className="text-xl font-semibold mb-4">Media Managers</h2>
          <div className="space-y-3">
            {data?.mediaManagers?.map((manager: any) => (
              <div key={manager.id} className="flex items-center justify-between p-4 bg-zinc-900 rounded-lg border border-zinc-800">
                <div>
                  <p className="font-medium">{manager.name}</p>
                  <p className="text-sm text-zinc-400 capitalize">{manager.type}</p>
                </div>
                <span className={`text-sm ${manager.online ? 'text-green-400' : 'text-red-400'}`}>
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

function StatCard({ title, value, icon: Icon }: { title: string; value: string | number; icon: any }) {
  return (
    <div className="bg-zinc-900 p-6 rounded-lg border border-zinc-800">
      <div className="flex items-center justify-between">
        <div>
          <p className="text-sm text-zinc-400">{title}</p>
          <p className="text-2xl font-bold mt-1">{value}</p>
        </div>
        <Icon className="h-8 w-8 text-zinc-600" />
      </div>
    </div>
  );
}
