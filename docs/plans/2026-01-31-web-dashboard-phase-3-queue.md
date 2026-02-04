# Phase 3: Queue - JellyWatch Web Dashboard

> **Parent Plan**: [Web Dashboard Foundation](../2026-01-31-web-dashboard.md)
> **Previous**: [Phase 2: Duplicates](./2026-01-31-web-dashboard-phase-2-duplicates.md)

**Goal**: Implement the Queue management page for monitoring and controlling download queues across Sonarr and Radarr.

**Dependencies**: Phase 0 (Foundation) + Phase 1 (Dashboard)

---

## Context Reference

### Established Patterns

**MediaManager Query Pattern**:
```typescript
const { data } = useManagerQueue(managerId);
const mutation = useClearQueueItem(managerId);
```

**Polling Strategy**: 10 seconds for active queue data, 30 seconds for stuck items.

**Types**:
```typescript
type QueueItem = {
  id: number;
  title: string;
  status: string;
  progress: number;
  size: number;
  sizeRemaining: number;
  timeLeft?: string;
  isStuck: boolean;
  errorMessage?: string;
};
```

---

## Phase 3 Tasks

### Task 3.1: Create Queue API Helpers

**Files**: `web/src/lib/api/client.ts`

```typescript
export const mediaManagerApi = {
  // ... existing methods
  getQueue: (id: string, limit?: number) =>
    api.get<QueueItem[]>(`/media-managers/${id}/queue`, { limit }),
  getStuck: (id: string) =>
    api.get<QueueItem[]>(`/media-managers/${id}/stuck`),
  clearItem: (managerId: string, itemId: number, blocklist?: boolean) =>
    api.delete(`/media-managers/${managerId}/queue/${itemId}`, { blocklist }),
  clearStuck: (managerId: string, blocklist?: boolean) =>
    api.delete(`/media-managers/${managerId}/stuck`, { blocklist }),
  retryItem: (managerId: string, itemId: number) =>
    api.post(`/media-managers/${managerId}/queue/${itemId}/retry`),
};
```

**Commit**: `git add web/src/lib/api/client.ts && git commit -m "feat: add queue API helpers"`

---

### Task 3.2: Create Queue Hooks

**Files**: `web/src/hooks/useQueue.ts`

```typescript
export function useManagerQueue(id: string, limit?: number) {
  return useQuery<QueueItem[]>({
    queryKey: queryKeys.mediaManagers.queue(id),
    queryFn: () => mediaManagerApi.getQueue(id, limit),
    enabled: !!id,
    refetchInterval: 10 * 1000, // 10 seconds for live data
  });
}

export function useClearQueueItem(managerId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ itemId, blocklist }: { itemId: number; blocklist?: boolean }) =>
      mediaManagerApi.clearItem(managerId, itemId, blocklist),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.mediaManagers.queue(managerId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.mediaManagers.stuck(managerId) });
      queryClient.invalidateQueries({ queryKey: queryKeys.dashboard });
    },
  });
}
```

**Commit**: `git add web/src/hooks/useQueue.ts && git commit -m "feat: add queue query hooks"`

---

### Task 3.3: Create QueueItem Component

**Files**: `web/src/components/features/queue/QueueItem.tsx`

```typescript
interface QueueItemProps {
  item: QueueItemType;
  onClear: (id: number) => void;
  onRetry: (id: number) => void;
}

export function QueueItem({ item, onClear, onRetry }: QueueItemProps) {
  return (
    <div className={cn(
      "flex items-center justify-between p-3 rounded-lg border",
      item.isStuck && "border-destructive/50 bg-destructive/5"
    )}>
      <div className="flex-1 min-w-0">
        <p className="font-medium truncate">{item.title}</p>
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          {item.status}
          {item.timeLeft && <span>• {item.timeLeft}</span>}
        </div>
        
        {/* Progress bar */}
        <div className="mt-2">
          <Progress value={item.progress} className="h-2" />
          <div className="flex justify-between text-xs text-muted-foreground mt-1">
            <span>{Math.round(item.progress)}%</span>
            <span>{formatBytes(item.sizeRemaining)} / {formatBytes(item.size)}</span>
          </div>
        </div>

        {/* Error message for stuck items */}
        {item.isStuck && item.errorMessage && (
          <p className="text-sm text-destructive mt-2">{item.errorMessage}</p>
        )}
      </div>

      <div className="flex items-center gap-2 ml-4">
        {item.isStuck ? (
          <>
            <Button size="sm" variant="outline" onClick={() => onRetry(item.id)}>
              Retry
            </Button>
            <Button size="sm" variant="destructive" onClick={() => onClear(item.id)}>
              Clear
            </Button>
          </>
        ) : (
          <Button size="sm" variant="ghost" onClick={() => onClear(item.id)}>
            Cancel
          </Button>
        )}
      </div>
    </div>
  );
}
```

**Commit**: `git add web/src/components/features/queue/QueueItem.tsx && git commit -m "feat: add QueueItem component with progress and actions"`

---

### Task 3.4: Create ManagerQueue Component

**Files**: `web/src/components/features/queue/ManagerQueue.tsx`

```typescript
interface ManagerQueueProps {
  managerId: string;
  managerName: string;
}

export function ManagerQueue({ managerId, managerName }: ManagerQueueProps) {
  const { data: queue, isLoading } = useManagerQueue(managerId);
  const { data: stuck } = useManagerStuck(managerId);
  const clearMutation = useClearQueueItem(managerId);
  const retryMutation = useRetryQueueItem(managerId);
  const clearStuckMutation = useClearStuckItems(managerId);

  const handleClearStuck = async () => {
    await clearStuckMutation.mutateAsync({ blocklist: false });
  };

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <CardTitle>{managerName}</CardTitle>
        {stuck && stuck.length > 0 && (
          <Button 
            size="sm" 
            variant="destructive" 
            onClick={handleClearStuck}
            disabled={clearStuckMutation.isPending}
          >
            Clear {stuck.length} Stuck
          </Button>
        )}
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <Skeleton className="h-32" />
        ) : queue && queue.length > 0 ? (
          <div className="space-y-3">
            {queue.map((item) => (
              <QueueItem
                key={item.id}
                item={item}
                onClear={(id) => clearMutation.mutate({ itemId: id })}
                onRetry={(id) => retryMutation.mutate(id)}
              />
            ))}
          </div>
        ) : (
          <p className="text-muted-foreground text-center py-8">Queue is empty</p>
        )}
      </CardContent>
    </Card>
  );
}
```

**Commit**: `git add web/src/components/features/queue/ManagerQueue.tsx && git commit -m "feat: add ManagerQueue component for single manager view"`

---

### Task 3.5: Create Queue Page

**Files**: `web/src/app/queue/page.tsx`

```typescript
export default function QueuePage() {
  const { data: managers } = useMediaManagers();

  return (
    <AppShell>
      <div className="space-y-6">
        <div>
          <h1 className="text-3xl font-bold">Queue</h1>
          <p className="text-muted-foreground">
            Monitor and manage download queues
          </p>
        </div>

        <div className="space-y-6">
          {managers?.map((manager) => (
            <ManagerQueue
              key={manager.id}
              managerId={manager.id}
              managerName={manager.name}
            />
          ))}
        </div>
      </div>
    </AppShell>
  );
}
```

**Commit**: `git add web/src/app/queue/page.tsx && git commit -m "feat: create queue page with multi-manager view"`

---

## Phase 3 Complete

**Summary**: Queue page with:
- ✅ Live polling (10s intervals)
- ✅ Progress bars for active downloads
- ✅ Stuck item detection and bulk clear
- ✅ Individual cancel/retry actions
- ✅ Multi-manager support

**Next**: [Phase 4: Activity](./2026-01-31-web-dashboard-phase-4-activity.md)
