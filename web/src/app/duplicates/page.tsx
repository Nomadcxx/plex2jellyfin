'use client';

import { AppShell } from '@/components/layout/AppShell';
import { DuplicateGroup } from '@/components/duplicates/DuplicateGroup';
import { useDuplicates } from '@/hooks/useDashboard';
import { AlertTriangle, Copy } from 'lucide-react';
import { formatBytes } from '@/lib/utils';

export default function DuplicatesPage() {
  const { data, isLoading, error } = useDuplicates();

  if (isLoading) {
    return (
      <AppShell>
        <div className="space-y-4">
          <h1 className="text-3xl font-bold">Duplicates</h1>
          <div className="h-32 bg-zinc-900 rounded-lg animate-pulse" />
          <div className="h-32 bg-zinc-900 rounded-lg animate-pulse" />
        </div>
      </AppShell>
    );
  }

  if (error) {
    return (
      <AppShell>
        <div className="p-4 bg-red-500/10 border border-red-500/30 rounded-lg">
          <p className="text-red-400">Failed to load duplicates</p>
        </div>
      </AppShell>
    );
  }

  const groups = data?.groups || [];
  const totalReclaimable = data?.reclaimableBytes || 0;

  return (
    <AppShell>
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold flex items-center gap-2">
              <Copy className="h-8 w-8" />
              Duplicates
            </h1>
            <p className="text-zinc-400 mt-1">
              {groups.length} duplicate groups found
              {totalReclaimable > 0 && (
                <span className="text-green-400 ml-2">
                  ({formatBytes(totalReclaimable)} reclaimable)
                </span>
              )}
            </p>
          </div>
        </div>

        {groups.length === 0 ? (
          <div className="p-8 bg-zinc-900 rounded-lg border border-zinc-800 text-center">
            <AlertTriangle className="h-12 w-12 text-zinc-600 mx-auto mb-4" />
            <p className="text-zinc-400">No duplicates found</p>
            <p className="text-sm text-zinc-500 mt-2">
              Run a scan to detect duplicate files
            </p>
          </div>
        ) : (
          <div className="space-y-4">
            {groups.map((group) => (
              <DuplicateGroup key={group.id} group={group} />
            ))}
          </div>
        )}
      </div>
    </AppShell>
  );
}
