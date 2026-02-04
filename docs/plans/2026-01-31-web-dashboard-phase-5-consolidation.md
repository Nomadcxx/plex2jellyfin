# Phase 5: Consolidation - JellyWatch Web Dashboard

> **Parent Plan**: [Web Dashboard Foundation](../2026-01-31-web-dashboard.md)
> **Previous**: [Phase 4: Activity](./2026-01-31-web-dashboard-phase-4-activity.md)

**Goal**: Implement scattered series consolidation page for merging TV series split across multiple drives.

---

## Phase 5 Tasks

### Task 5.1: Create Consolidation API Helpers

**Files**: `web/src/lib/api/client.ts`

```typescript
export const scatteredApi = {
  getScattered: () => api.get<ScatteredAnalysis>('/scattered'),
  consolidateItem: (itemId: number, dryRun?: boolean) =>
    api.post<ConsolidationResult>(`/scattered/${itemId}/consolidate`, { dryRun }),
  consolidateAll: (dryRun?: boolean) =>
    api.post<OperationResult>('/scattered/consolidate-all', { dryRun }),
};
```

**Commit**: `git add web/src/lib/api/client.ts && git commit -m "feat: add consolidation API helpers"`

---

### Task 5.2: Create Consolidation Hooks

**Files**: `web/src/hooks/useConsolidation.ts`

```typescript
export function useScattered() {
  return useQuery<ScatteredAnalysis>({
    queryKey: ['scattered'],
    queryFn: scatteredApi.getScattered,
    refetchInterval: 60 * 1000,
  });
}

export function useConsolidateItem() {
  const queryClient = useQueryClient();
  return useMutation<ConsolidationResult, Error, { itemId: number; dryRun?: boolean }>({
    mutationFn: ({ itemId, dryRun }) => scatteredApi.consolidateItem(itemId, dryRun),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['scattered'] });
      queryClient.invalidateQueries({ queryKey: ['dashboard'] });
    },
  });
}
```

**Commit**: `git add web/src/hooks/useConsolidation.ts && git commit -m "feat: add consolidation hooks"`

---

### Task 5.3: Create ScatteredItem Component

**Files**: `web/src/components/features/consolidation/ScatteredItem.tsx`

```typescript
interface ScatteredItemProps {
  item: ScatteredItemType;
  onConsolidate: (id: number, dryRun: boolean) => void;
  isConsolidating: boolean;
}

export function ScatteredItemCard({ item, onConsolidate, isConsolidating }: ScatteredItemProps) {
  const [showPreview, setShowPreview] = useState(false);

  return (
    <Card>
      <CardHeader>
        <CardTitle>
          {item.title} {item.year && `(${item.year})`}
        </CardTitle>
        <CardDescription>
          {item.locations.length} locations • {item.filesToMove} files • {formatBytes(item.bytesToMove)}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="space-y-2">
          {item.seasons.map((season) => (
            <div
              key={season.season}
              className={cn(
                'flex items-center justify-between p-2 rounded',
                season.needsMove ? 'bg-amber-500/10' : 'bg-green-500/10'
              )}
            >
              <span>Season {season.season}</span>
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <span>{season.location}</span>
                {season.needsMove && (
                  <ArrowRight className="h-4 w-4 text-amber-500" />
                )}
              </div>
            </div>
          ))}
        </div>

        <div className="flex gap-2 mt-4">
          <Button
            variant="outline"
            onClick={() => setShowPreview(true)}
            disabled={isConsolidating}
          >
            Preview
          </Button>
          <Button
            onClick={() => onConsolidate(item.id, false)}
            disabled={isConsolidating}
          >
            Consolidate
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
```

**Commit**: `git add web/src/components/features/consolidation/ScatteredItem.tsx && git commit -m "feat: add ScatteredItem component"`

---

### Task 5.4: Create Consolidation Page

**Files**: `web/src/app/consolidation/page.tsx`

```typescript
export default function ConsolidationPage() {
  const { data, isLoading } = useScattered();
  const consolidateMutation = useConsolidateItem();

  return (
    <AppShell>
      <div className="space-y-6">
        <div>
          <h1 className="text-3xl font-bold">Consolidation</h1>
          <p className="text-muted-foreground">
            Merge TV series scattered across multiple drives
          </p>
        </div>

        {data && data.totalItems > 0 && (
          <Card className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="font-medium">{data.totalItems} series need consolidation</p>
                <p className="text-sm text-muted-foreground">
                  {data.totalMoves} moves • {formatBytes(data.totalBytes)}
                </p>
              </div>
              <Button>Consolidate All</Button>
            </div>
          </Card>
        )}

        {isLoading ? (
          <Skeleton className="h-64" />
        ) : data?.items.length === 0 ? (
          <p className="text-muted-foreground text-center py-8">
            All series are properly consolidated
          </p>
        ) : (
          <div className="space-y-4">
            {data?.items.map((item) => (
              <ScatteredItemCard
                key={item.id}
                item={item}
                onConsolidate={consolidateMutation.mutateAsync}
                isConsolidating={consolidateMutation.isPending}
              />
            ))}
          </div>
        )}
      </div>
    </AppShell>
  );
}
```

**Commit**: `git add web/src/app/consolidation/page.tsx && git commit -m "feat: create consolidation page"`

---

## Phase 5 Complete

**Next**: [Phase 6: Auth & Polish](./2026-01-31-web-dashboard-phase-6-auth.md)
