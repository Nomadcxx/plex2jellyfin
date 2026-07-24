'use client';

import { useState } from 'react';
import { AppShell } from '@/components/layout/AppShell';
import { DuplicateGroup } from '@/components/duplicates/DuplicateGroup';
import { useDuplicates } from '@/hooks/useDashboard';
import { AlertTriangle, Copy, Search } from 'lucide-react';
import { formatBytes } from '@/lib/utils';
import { Input } from '@/components/ui/input';
import { filterDuplicateGroups, sortDuplicateGroups, type DuplicateSort } from './duplicateFilters';

export default function DuplicatesPage() {
  const { data, isLoading, error } = useDuplicates();
  const [query, setQuery] = useState('');
  const [sort, setSort] = useState<DuplicateSort>('reclaimable');

  if (isLoading) {
    return (
      <AppShell>
        <div className="space-y-4">
          <h1 className="text-3xl font-bold">Duplicates</h1>
          <div className="h-32 vision-card animate-pulse" />
          <div className="h-32 vision-card animate-pulse" />
        </div>
      </AppShell>
    );
  }

  if (error) {
    return (
      <AppShell>
        <div className="p-4 bg-terminal-red/10 border border-terminal-red/30 rounded-lg">
          <p className="text-terminal-red">Failed to load duplicates</p>
        </div>
      </AppShell>
    );
  }

  const groups = data?.groups || [];
  const totalReclaimable = data?.reclaimableBytes || 0;
  const visibleGroups = sortDuplicateGroups(filterDuplicateGroups(groups, query), sort);

  return (
    <AppShell>
      <div className="space-y-5">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
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
          <div className="grid gap-2 sm:grid-cols-[minmax(220px,360px)_160px]">
            <label className="relative">
              <Search className="pointer-events-none absolute left-3 top-2.5 h-4 w-4 text-zinc-500" />
              <Input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                className="pl-9"
                placeholder="Search title or path"
              />
            </label>
            <select
              value={sort}
              onChange={(e) => setSort(e.target.value as DuplicateSort)}
              className="h-10 rounded-md border border-zinc-700/50 bg-zinc-900/40 px-3 text-sm text-zinc-100 backdrop-blur-sm transition-colors focus:outline-none focus:ring-2 focus:ring-amber-500/40 focus:border-amber-500/30"
              aria-label="Sort duplicates"
            >
              <option value="reclaimable">Most reclaimable</option>
              <option value="files">Most files</option>
              <option value="title">Title</option>
            </select>
          </div>
        </div>

        {groups.length === 0 ? (
          <div className="vision-card-secondary p-8 text-center">
            <AlertTriangle className="h-12 w-12 text-zinc-600 mx-auto mb-4" />
            <p className="text-zinc-400">No duplicates found</p>
            <p className="text-sm text-zinc-500 mt-2">
              Run a scan to detect duplicate files
            </p>
          </div>
        ) : visibleGroups.length === 0 ? (
          <div className="vision-card-secondary p-6 text-sm text-zinc-400">
            No duplicate groups match the current filter.
          </div>
        ) : (
          <div className="space-y-3">
            <p className="text-xs text-zinc-500">
              Showing {visibleGroups.length} of {groups.length} groups
            </p>
            {visibleGroups.map((group) => (
              <DuplicateGroup key={group.id} group={group} />
            ))}
          </div>
        )}
      </div>
    </AppShell>
  );
}
