# Phase 2: Duplicates - JellyWatch Web Dashboard

> **Parent Plan**: [Web Dashboard Foundation](../2026-01-31-web-dashboard.md)

**Goal**: Implement the Duplicates management page, allowing users to view duplicate file groups, compare quality scores, and delete inferior copies to reclaim storage.

**Dependencies**: Phase 0 (Foundation) + Phase 1 (Dashboard) must be complete.

---

## Context Reference

### From Foundation Plan

**Architecture**: Next.js static export embedded in Go binary, TanStack Query for data, shadcn/ui components, dark-only theme.

**Patterns Established**:
- API client: `web/src/lib/api/client.ts`
- Query keys: `web/src/lib/queryKeys.ts`
- Data fetching: 60s stale time, 30s polling for active data
- Mutations: Invalidate related queries on success
- Error handling: Toast notifications for user feedback

**Backend Abstractions**:
- `MediaManager` interface: `internal/mediamanager/interface.go`
- Duplicate analysis already exists: `internal/api/handlers.go` has `GetDuplicates`

**Types** (from `web/src/types/api.ts`):
```typescript
type DuplicateAnalysis = {
  groups: DuplicateGroup[];
  totalFiles: number;
  totalGroups: number;
  reclaimableBytes: number;
};

type DuplicateGroup = {
  id: string;
  title: string;
  year?: number;
  mediaType: 'movie' | 'series';
  season?: number;
  episode?: number;
  files: MediaFile[];
  bestFileId: number;
  reclaimableBytes: number;
};

type MediaFile = {
  id: number;
  path: string;
  size: number;
  resolution?: string;
  sourceType?: string;
  codec?: string;
  qualityScore: number;
  createdAt: string;
};
```

---

## Phase 2 Tasks

### Task 2.1: Implement Duplicates API Helpers

**Files**:
- Modify: `web/src/lib/api/client.ts` (add duplicatesApi)

**Step 1: Add duplicates API methods**

```typescript
// Add to web/src/lib/api/client.ts

export const duplicatesApi = {
  getDuplicates: () => api.get<import('@/types').DuplicateAnalysis>('/duplicates'),
  
  deleteDuplicate: (groupId: string, fileId: number) =>
    api.delete<import('@/types').OperationResult>(`/duplicates/${groupId}`, { fileId }),
  
  batchDelete: (deletions: { groupId: string; fileId: number }[]) =>
    api.post<import('@/types').OperationResult>('/duplicates/batch', { deletions }),
  
  resolveAll: (dryRun?: boolean) =>
    api.post<import('@/types').OperationResult>('/duplicates/resolve-all', { dryRun }),
};
```

**Step 2: Test compilation**

Run: `cd web && npm run typecheck`
Expected: No errors

**Step 3: Commit**

```bash
git add web/src/lib/api/client.ts
git commit -m "feat: add duplicates API helpers"
```

---

### Task 2.2: Create Duplicates Query Hooks

**Files**:
- Create: `web/src/hooks/useDuplicates.ts`

**Step 1: Write hooks**

```typescript
// web/src/hooks/useDuplicates.ts
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { queryKeys } from '@/lib/queryKeys';
import { duplicatesApi } from '@/lib/api/client';
import type { DuplicateAnalysis, OperationResult } from '@/types';

export function useDuplicates() {
  return useQuery<DuplicateAnalysis>({
    queryKey: queryKeys.duplicates,
    queryFn: duplicatesApi.getDuplicates,
    refetchInterval: 60 * 1000, // 1 minute
  });
}

export function useDeleteDuplicate() {
  const queryClient = useQueryClient();

  return useMutation<OperationResult, Error, { groupId: string; fileId: number }>({
    mutationFn: ({ groupId, fileId }) =>
      duplicatesApi.deleteDuplicate(groupId, fileId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.duplicates });
      queryClient.invalidateQueries({ queryKey: queryKeys.dashboard });
    },
  });
}

export function useBatchDeleteDuplicates() {
  const queryClient = useQueryClient();

  return useMutation<OperationResult, Error, { groupId: string; fileId: number }[]>({
    mutationFn: (deletions) => duplicatesApi.batchDelete(deletions),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.duplicates });
      queryClient.invalidateQueries({ queryKey: queryKeys.dashboard });
    },
  });
}

export function useResolveAllDuplicates() {
  const queryClient = useQueryClient();

  return useMutation<OperationResult, Error, boolean>({
    mutationFn: (dryRun) => duplicatesApi.resolveAll(dryRun),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.duplicates });
      queryClient.invalidateQueries({ queryKey: queryKeys.dashboard });
    },
  });
}
```

**Step 2: Export from hooks index**

```typescript
// Add to web/src/hooks/index.ts
export * from './useDuplicates';
```

**Step 3: Commit**

```bash
git add web/src/hooks/useDuplicates.ts web/src/hooks/index.ts
git commit -m "feat: add duplicates query hooks with mutations"
```

---

### Task 2.3: Create Quality Badge Component

**Files**:
- Create: `web/src/components/ui/QualityBadge.tsx`

**Step 1: Write component**

```typescript
// web/src/components/ui/QualityBadge.tsx
import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';

interface QualityBadgeProps {
  sourceType?: string;
  resolution?: string;
  className?: string;
}

const qualityColors: Record<string, string> = {
  // Source types
  'REMUX': 'bg-purple-500/15 text-purple-500 border-purple-500/50',
  'BluRay': 'bg-blue-500/15 text-blue-500 border-blue-500/50',
  'BDRip': 'bg-blue-500/15 text-blue-500 border-blue-500/50',
  'WEB-DL': 'bg-green-500/15 text-green-500 border-green-500/50',
  'WEBRip': 'bg-green-500/15 text-green-500 border-green-500/50',
  'HDTV': 'bg-yellow-500/15 text-yellow-500 border-yellow-500/50',
  'DVDRip': 'bg-orange-500/15 text-orange-500 border-orange-500/50',
  // Resolutions
  '2160p': 'bg-emerald-500/15 text-emerald-500',
  '1080p': 'bg-cyan-500/15 text-cyan-500',
  '720p': 'bg-slate-500/15 text-slate-500',
};

export function QualityBadge({ sourceType, resolution, className }: QualityBadgeProps) {
  const display = sourceType || resolution || 'Unknown';
  const colorClass = qualityColors[sourceType || ''] || 
                    qualityColors[resolution || ''] || 
                    'bg-muted text-muted-foreground';

  return (
    <Badge variant="outline" className={cn(colorClass, className)}>
      {display}
    </Badge>
  );
}
```

**Step 2: Commit**

```bash
git add web/src/components/ui/QualityBadge.tsx
git commit -m "feat: add QualityBadge component for source/resolution display"
```

---

### Task 2.4: Create FileSize Component

**Files**:
- Create: `web/src/components/ui/FileSize.tsx`

**Step 1: Write component**

```typescript
// web/src/components/ui/FileSize.tsx
import { formatBytes } from '@/lib/utils';
import { cn } from '@/lib/utils';

interface FileSizeProps {
  bytes: number;
  className?: string;
  showComparison?: number; // Another size to compare against
}

export function FileSize({ bytes, className, showComparison }: FileSizeProps) {
  const formatted = formatBytes(bytes);
  
  if (!showComparison) {
    return <span className={cn('text-sm tabular-nums', className)}>{formatted}</span>;
  }

  const diff = bytes - showComparison;
  const diffFormatted = diff > 0 ? `+${formatBytes(diff)}` : formatBytes(Math.abs(diff));
  const isLarger = diff > 0;

  return (
    <div className={cn('text-sm', className)}>
      <span className="tabular-nums">{formatted}</span>
      <span className={cn(
        'ml-2 text-xs',
        isLarger ? 'text-red-400' : 'text-green-400'
      )}>
        ({isLarger ? '+' : '-'}{diffFormatted})
      </span>
    </div>
  );
}
```

**Step 2: Commit**

```bash
git add web/src/components/ui/FileSize.tsx
git commit -m "feat: add FileSize component with comparison support"
```

---

### Task 2.5: Create DuplicateGroup Component

**Files**:
- Create: `web/src/components/features/duplicates/DuplicateGroup.tsx`

**Step 1: Write component**

```typescript
// web/src/components/features/duplicates/DuplicateGroup.tsx
'use client';

import { useState } from 'react';
import { Card, CardContent, CardHeader } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { QualityBadge } from '@/components/ui/QualityBadge';
import { FileSize } from '@/components/ui/FileSize';
import { Trash2, Star, ChevronDown, ChevronRight } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { DuplicateGroup as DuplicateGroupType } from '@/types';

interface DuplicateGroupProps {
  group: DuplicateGroupType;
  onDelete: (groupId: string, fileId: number) => void;
  isDeleting?: boolean;
}

export function DuplicateGroup({ group, onDelete, isDeleting }: DuplicateGroupProps) {
  const [isOpen, setIsOpen] = useState(false);

  const bestFile = group.files.find(f => f.id === group.bestFileId);
  const inferiorFiles = group.files.filter(f => f.id !== group.bestFileId);

  return (
    <Card>
      <Collapsible open={isOpen} onOpenChange={setIsOpen}>
        <CardHeader className="pb-3">
          <CollapsibleTrigger className="flex items-center justify-between w-full text-left hover:no-underline">
            <div className="flex items-center gap-3">
              {isOpen ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
              <div>
                <h3 className="font-semibold">
                  {group.title}
                  {group.year && ` (${group.year})`}
                </h3>
                <p className="text-sm text-muted-foreground">
                  {group.files.length} files • <FileSize bytes={group.reclaimableBytes} /> reclaimable
                </p>
              </div>
            </div>
            <QualityBadge sourceType={bestFile?.sourceType} resolution={bestFile?.resolution} />
          </CollapsibleTrigger>
        </CardHeader>

        <CollapsibleContent>
          <CardContent className="pt-0">
            <div className="space-y-2">
              {/* Best file */}
              {bestFile && (
                <div className="flex items-center justify-between p-3 rounded-lg bg-secondary/50 border border-secondary">
                  <div className="flex items-center gap-3">
                    <Star className="h-4 w-4 text-yellow-500 fill-yellow-500" />
                    <div>
                      <p className="text-sm font-medium truncate max-w-md">
                        {bestFile.path.split('/').pop()}
                      </p>
                      <div className="flex items-center gap-2 mt-1">
                        <QualityBadge sourceType={bestFile.sourceType} resolution={bestFile.resolution} />
                        <span className="text-xs text-muted-foreground">Score: {bestFile.qualityScore}</span>
                      </div>
                    </div>
                  </div>
                  <FileSize bytes={bestFile.size} />
                </div>
              )}

              {/* Inferior files */}
              {inferiorFiles.map((file) => (
                <div
                  key={file.id}
                  className="flex items-center justify-between p-3 rounded-lg border border-border hover:bg-secondary/30 transition-colors"
                >
                  <div className="flex items-center gap-3">
                    <div className="w-4" /> {/* Spacer to align with best file */}
                    <div>
                      <p className="text-sm truncate max-w-md">
                        {file.path.split('/').pop()}
                      </p>
                      <div className="flex items-center gap-2 mt-1">
                        <QualityBadge sourceType={file.sourceType} resolution={file.resolution} />
                        <span className="text-xs text-muted-foreground">Score: {file.qualityScore}</span>
                      </div>
                    </div>
                  </div>
                  <div className="flex items-center gap-3">
                    <FileSize bytes={file.size} showComparison={bestFile?.size} />
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8 text-destructive hover:text-destructive hover:bg-destructive/10"
                      onClick={() => onDelete(group.id, file.id)}
                      disabled={isDeleting}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </CollapsibleContent>
      </Collapsible>
    </Card>
  );
}
```

**Step 2: Commit**

```bash
git add web/src/components/features/duplicates/DuplicateGroup.tsx
git commit -m "feat: add DuplicateGroup component with collapsible file list"
```

---

### Task 2.6: Create DuplicatesList Component

**Files**:
- Create: `web/src/components/features/duplicates/DuplicatesList.tsx`

**Step 1: Write component**

```typescript
// web/src/components/features/duplicates/DuplicatesList.tsx
'use client';

import { DuplicateGroup } from './DuplicateGroup';
import { Skeleton } from '@/components/ui/skeleton';
import { useToast } from '@/components/ui/use-toast';
import type { DuplicateAnalysis } from '@/types';

interface DuplicatesListProps {
  data?: DuplicateAnalysis;
  isLoading: boolean;
  onDelete: (groupId: string, fileId: number) => Promise<void>;
  isDeleting: boolean;
}

export function DuplicatesList({ data, isLoading, onDelete, isDeleting }: DuplicatesListProps) {
  const { toast } = useToast();

  const handleDelete = async (groupId: string, fileId: number) => {
    try {
      await onDelete(groupId, fileId);
      toast({
        title: 'File deleted',
        description: 'The duplicate file has been removed.',
      });
    } catch (error) {
      toast({
        title: 'Error',
        description: error instanceof Error ? error.message : 'Failed to delete file',
        variant: 'destructive',
      });
    }
  };

  if (isLoading) {
    return (
      <div className="space-y-4">
        {Array(3).fill(0).map((_, i) => (
          <Skeleton key={i} className="h-32" />
        ))}
      </div>
    );
  }

  if (!data || data.totalGroups === 0) {
    return (
      <div className="text-center py-12">
        <p className="text-muted-foreground">No duplicates found</p>
        <p className="text-sm text-muted-foreground mt-1">
          Your library is clean!
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {data.groups.map((group) => (
        <DuplicateGroup
          key={group.id}
          group={group}
          onDelete={handleDelete}
          isDeleting={isDeleting}
        />
      ))}
    </div>
  );
}
```

**Step 2: Commit**

```bash
git add web/src/components/features/duplicates/DuplicatesList.tsx
git commit -m "feat: add DuplicatesList component with loading and empty states"
```

---

### Task 2.7: Create BatchActions Component

**Files**:
- Create: `web/src/components/features/duplicates/BatchActions.tsx`

**Step 1: Write component**

```typescript
// web/src/components/features/duplicates/BatchActions.tsx
'use client';

import { useState } from 'react';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { AlertTriangle } from 'lucide-react';
import { formatBytes } from '@/lib/utils';

interface BatchActionsProps {
  totalGroups: number;
  reclaimableBytes: number;
  onResolveAll: (dryRun: boolean) => Promise<void>;
  isResolving: boolean;
}

export function BatchActions({
  totalGroups,
  reclaimableBytes,
  onResolveAll,
  isResolving,
}: BatchActionsProps) {
  const [showConfirm, setShowConfirm] = useState(false);
  const [showDryRun, setShowDryRun] = useState(false);

  const handleDryRun = async () => {
    await onResolveAll(true);
    setShowDryRun(true);
  };

  const handleConfirm = async () => {
    await onResolveAll(false);
    setShowConfirm(false);
  };

  if (totalGroups === 0) return null;

  return (
    <>
      <div className="flex items-center justify-between p-4 rounded-lg border bg-card">
        <div>
          <p className="font-medium">{totalGroups} duplicate groups found</p>
          <p className="text-sm text-muted-foreground">
            {formatBytes(reclaimableBytes)} can be reclaimed
          </p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" onClick={handleDryRun} disabled={isResolving}>
            Preview
          </Button>
          <Button onClick={() => setShowConfirm(true)} disabled={isResolving}>
            Resolve All
          </Button>
        </div>
      </div>

      {/* Confirmation Dialog */}
      <Dialog open={showConfirm} onOpenChange={setShowConfirm}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <AlertTriangle className="h-5 w-5 text-destructive" />
              Confirm Bulk Delete
            </DialogTitle>
            <DialogDescription>
              This will delete {totalGroups} inferior duplicate files, keeping only
              the best quality version of each. This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowConfirm(false)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleConfirm} disabled={isResolving}>
              {isResolving ? 'Deleting...' : 'Delete All Inferior Files'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
```

**Step 2: Commit**

```bash
git add web/src/components/features/duplicates/BatchActions.tsx
git commit -m "feat: add BatchActions component for bulk duplicate resolution"
```

---

### Task 2.8: Create Duplicates Page

**Files**:
- Create: `web/src/app/duplicates/page.tsx`

**Step 1: Write page**

```typescript
// web/src/app/duplicates/page.tsx
'use client';

import { AppShell } from '@/components/layout/AppShell';
import { DuplicatesList } from '@/components/features/duplicates/DuplicatesList';
import { BatchActions } from '@/components/features/duplicates/BatchActions';
import {
  useDuplicates,
  useDeleteDuplicate,
  useResolveAllDuplicates,
} from '@/hooks/useDuplicates';
import { Button } from '@/components/ui/button';
import { RefreshCw } from 'lucide-react';

export default function DuplicatesPage() {
  const { data, isLoading, refetch } = useDuplicates();
  const deleteMutation = useDeleteDuplicate();
  const resolveMutation = useResolveAllDuplicates();

  return (
    <AppShell>
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold">Duplicates</h1>
            <p className="text-muted-foreground">
              Find and remove duplicate files to reclaim storage
            </p>
          </div>
          <Button
            variant="outline"
            size="icon"
            onClick={() => refetch()}
            disabled={isLoading}
          >
            <RefreshCw className={cn('h-4 w-4', isLoading && 'animate-spin')} />
          </Button>
        </div>

        {data && (
          <BatchActions
            totalGroups={data.totalGroups}
            reclaimableBytes={data.reclaimableBytes}
            onResolveAll={resolveMutation.mutateAsync}
            isResolving={resolveMutation.isPending}
          />
        )}

        <DuplicatesList
          data={data}
          isLoading={isLoading}
          onDelete={deleteMutation.mutateAsync}
          isDeleting={deleteMutation.isPending}
        />
      </div>
    </AppShell>
  );
}
```

**Step 2: Test compilation**

Run: `cd web && npm run typecheck`
Expected: No errors

**Step 3: Commit**

```bash
git add web/src/app/duplicates/page.tsx
git commit -m "feat: create duplicates page with list and batch actions"
```

---

### Task 2.9: Add Duplicates Navigation Link

**Files**:
- Modify: `web/src/components/layout/Sidebar.tsx`

**Step 1: Already exists** - The Sidebar from Phase 1 already includes:
```typescript
{ name: 'Duplicates', href: '/duplicates', icon: Copy },
```

**Verification**: Confirm navigation works
Run: `cd web && npm run build`
Expected: Build succeeds with no route errors

---

### Task 2.10: Create Duplicates E2E Test

**Files**:
- Create: `web/e2e/duplicates.spec.ts`

**Step 1: Write test**

```typescript
// web/e2e/duplicates.spec.ts
import { test, expect } from '@playwright/test';

test.describe('Duplicates Page', () => {
  test.beforeEach(async ({ page }) => {
    // Mock API response
    await page.route('/api/v1/duplicates', async (route) => {
      await route.fulfill({
        json: {
          groups: [
            {
              id: 'test-movie-2024',
              title: 'Test Movie',
              year: 2024,
              mediaType: 'movie',
              files: [
                {
                  id: 1,
                  path: '/disk1/Movies/Test Movie (2024).mkv',
                  size: 45000000000,
                  resolution: '2160p',
                  sourceType: 'REMUX',
                  qualityScore: 95,
                  createdAt: '2024-01-01T00:00:00Z',
                },
                {
                  id: 2,
                  path: '/disk2/Movies/Test Movie (2024).mkv',
                  size: 12000000000,
                  resolution: '1080p',
                  sourceType: 'BluRay',
                  qualityScore: 75,
                  createdAt: '2024-01-02T00:00:00Z',
                },
              ],
              bestFileId: 1,
              reclaimableBytes: 12000000000,
            },
          ],
          totalFiles: 2,
          totalGroups: 1,
          reclaimableBytes: 12000000000,
        },
      });
    });

    await page.goto('/duplicates');
  });

  test('displays duplicate groups', async ({ page }) => {
    await expect(page.getByText('Test Movie (2024)')).toBeVisible();
    await expect(page.getByText('1 duplicate groups found')).toBeVisible();
    await expect(page.getByText('11.2 GB can be reclaimed')).toBeVisible();
  });

  test('expands group to show files', async ({ page }) => {
    await page.getByText('Test Movie (2024)').click();
    await expect(page.getByText('REMUX')).toBeVisible();
    await expect(page.getByText('BluRay')).toBeVisible();
    await expect(page.getByText('Score: 95')).toBeVisible();
  });

  test('shows best file indicator', async ({ page }) => {
    await page.getByText('Test Movie (2024)').click();
    // Best file should have star icon
    await expect(page.locator('.fill-yellow-500')).toBeVisible();
  });

  test('can delete duplicate file', async ({ page }) => {
    // Mock delete endpoint
    await page.route('/api/v1/duplicates/**', async (route) => {
      if (route.request().method() === 'DELETE') {
        await route.fulfill({
          json: { success: true, message: 'File deleted' },
        });
      }
    });

    await page.getByText('Test Movie (2024)').click();
    await page.getByRole('button', { name: /delete/i }).first().click();

    // Verify success toast
    await expect(page.getByText('File deleted')).toBeVisible();
  });
});
```

**Step 2: Commit**

```bash
git add web/e2e/duplicates.spec.ts
git commit -m "test: add E2E tests for duplicates page"
```

---

## Phase 2 Complete

**Summary**: Duplicates page implementation with:
- ✅ API helpers for duplicates operations
- ✅ TanStack Query hooks with mutations
- ✅ QualityBadge and FileSize UI components
- ✅ DuplicateGroup component with collapsible view
- ✅ DuplicatesList with loading/empty states
- ✅ BatchActions for bulk operations
- ✅ Full page integration
- ✅ E2E tests

**Integration Points**:
- Invalidates `queryKeys.duplicates` and `queryKeys.dashboard` on mutations
- Uses `AppShell` layout from Phase 1
- Uses `useToast` for user feedback (from shadcn/ui)
- Links to existing `/api/v1/duplicates` endpoint

**Next**: Proceed to [Phase 3: Queue](./2026-01-31-web-dashboard-phase-3-queue.md)

---

**Phase Status**: READY FOR EXECUTION
