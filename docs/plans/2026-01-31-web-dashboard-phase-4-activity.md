# Phase 4: Activity - JellyWatch Web Dashboard

> **Parent Plan**: [Web Dashboard Foundation](../2026-01-31-web-dashboard.md)
> **Previous**: [Phase 3: Queue](./2026-01-31-web-dashboard-phase-3-queue.md)

**Goal**: Real-time activity stream via Server-Sent Events (SSE) with pagination for historical events.

---

## Phase 4 Tasks

### Task 4.1: Create SSE Activity Stream Hook

**Files**: `web/src/hooks/useActivityStream.ts`

```typescript
export function useActivityStream(enabled: boolean = true) {
  const [events, setEvents] = useState<ActivityEvent[]>([]);
  const [isConnected, setIsConnected] = useState(false);
  const queryClient = useQueryClient();

  useEffect(() => {
    if (!enabled) return;

    const eventSource = new EventSource('/api/v1/activity/stream');

    eventSource.onopen = () => setIsConnected(true);
    
    eventSource.onmessage = (e) => {
      const event: ActivityEvent = JSON.parse(e.data);
      setEvents(prev => [event, ...prev].slice(0, 100));
      
      // Invalidate related queries based on event type
      if (event.type === 'DUPLICATE_FOUND') {
        queryClient.invalidateQueries({ queryKey: ['duplicates'] });
      }
    };

    eventSource.onerror = () => {
      setIsConnected(false);
      setTimeout(() => {
        queryClient.invalidateQueries({ queryKey: ['activity'] });
      }, 5000);
    };

    return () => eventSource.close();
  }, [enabled, queryClient]);

  return { events, isConnected };
}
```

**Commit**: `git add web/src/hooks/useActivityStream.ts && git commit -m "feat: add SSE activity stream hook"`

---

### Task 4.2: Create ActivityEvent Component

**Files**: `web/src/components/features/activity/ActivityEvent.tsx`

```typescript
interface ActivityEventProps {
  event: ActivityEventType;
}

const eventIcons = {
  FILE_DETECTED: FileText,
  FILE_ORGANIZED: CheckCircle,
  DUPLICATE_FOUND: Copy,
  SYNC_COMPLETED: RefreshCw,
  ERROR: AlertCircle,
};

const eventColors = {
  FILE_ORGANIZED: 'text-green-500',
  DUPLICATE_FOUND: 'text-blue-500',
  ERROR: 'text-destructive',
};

export function ActivityEventItem({ event }: ActivityEventProps) {
  const Icon = eventIcons[event.type] || FileText;
  const colorClass = eventColors[event.type as keyof typeof eventColors] || 'text-muted-foreground';

  return (
    <div className="flex items-start gap-3 py-3 border-b border-border last:border-0">
      <Icon className={cn('h-4 w-4 mt-0.5', colorClass)} />
      <div className="flex-1 min-w-0">
        <p className="text-sm">{event.message}</p>
        <p className="text-xs text-muted-foreground mt-1">
          {formatRelativeTime(event.timestamp)}
        </p>
      </div>
    </div>
  );
}
```

**Commit**: `git add web/src/components/features/activity/ActivityEvent.tsx && git commit -m "feat: add ActivityEvent component"`

---

### Task 4.3: Create Activity Page

**Files**: `web/src/app/activity/page.tsx`

```typescript
export default function ActivityPage() {
  const [isPaused, setIsPaused] = useState(false);
  const { events, isConnected } = useActivityStream(!isPaused);

  return (
    <AppShell>
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold">Activity</h1>
            <p className="text-muted-foreground">
              Real-time event stream
            </p>
          </div>
          <div className="flex items-center gap-4">
            <StatusBadge 
              status={isConnected ? 'online' : 'offline'} 
              text={isConnected ? 'Live' : 'Disconnected'} 
            />
            <Button
              variant="outline"
              size="sm"
              onClick={() => setIsPaused(!isPaused)}
            >
              {isPaused ? 'Resume' : 'Pause'}
            </Button>
          </div>
        </div>

        <ScrollArea className="h-[calc(100vh-200px)]">
          <div className="space-y-1 pr-4">
            {events.map((event) => (
              <ActivityEventItem key={event.id} event={event} />
            ))}
            {events.length === 0 && (
              <p className="text-muted-foreground text-center py-8">
                No activity yet
              </p>
            )}
          </div>
        </ScrollArea>
      </div>
    </AppShell>
  );
}
```

**Commit**: `git add web/src/app/activity/page.tsx && git commit -m "feat: create activity page with live stream"`

---

## Phase 4 Complete

**Next**: [Phase 5: Consolidation](./2026-01-31-web-dashboard-phase-5-consolidation.md)
