'use client';

import { AppShell } from '@/components/layout/AppShell';
import { QueueItem } from '@/components/queue/QueueItem';
import { useMediaManagers } from '@/hooks/useDashboard';
import { useQueue, useStuckItems } from '@/hooks/useQueue';
import { Download, AlertTriangle } from 'lucide-react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Badge } from '@/components/ui/badge';
import { Alert, AlertDescription } from '@/components/ui/alert';

export default function QueuePage() {
  const { data: managersData } = useMediaManagers();
  const managers = managersData || [];

  // Jellyfin has no download queue — only pick Sonarr/Radarr-style managers.
  const activeManager = managers.find(m => m.online && m.type !== 'jellyfin');
  const managerId = activeManager?.id;

  const { data: queueData, isLoading: queueLoading, isError: queueError, error: queueErr } = useQueue(managerId || '');
  const { data: stuckData, isLoading: stuckLoading, isError: stuckError, error: stuckErr } = useStuckItems(managerId || '');

  const queueItems = queueData?.items || [];
  const stuckItems = stuckData?.items || [];

  return (
    <AppShell>
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold flex items-center gap-2">
              <Download className="h-8 w-8" />
              Download Queue
            </h1>
            {activeManager && (
              <p className="text-zinc-400 mt-1">
                {activeManager.name} ({activeManager.type})
              </p>
            )}
          </div>
        </div>

        {!managerId ? (
          <div className="p-8 bg-zinc-900 rounded-lg border border-zinc-800 text-center">
            <AlertTriangle className="h-12 w-12 text-zinc-600 mx-auto mb-4" />
            <p className="text-zinc-400">No active media manager</p>
            <p className="text-sm text-zinc-500 mt-2">
              Configure Sonarr/Radarr in settings
            </p>
          </div>
        ) : (
          <Tabs defaultValue="active">
            <TabsList>
              <TabsTrigger value="active">
                Active
                {queueItems.length > 0 && (
                  <Badge variant="secondary" className="ml-2">
                    {queueItems.length}
                  </Badge>
                )}
              </TabsTrigger>
              <TabsTrigger value="stuck">
                Stuck
                {stuckItems.length > 0 && (
                  <Badge variant="destructive" className="ml-2">
                    {stuckItems.length}
                  </Badge>
                )}
              </TabsTrigger>
            </TabsList>

            <TabsContent value="active" className="space-y-4">
              {queueLoading ? (
                <div className="h-32 bg-zinc-900 rounded-lg animate-pulse" />
              ) : queueError ? (
                <Alert variant="destructive">
                  <AlertTriangle className="h-4 w-4" />
                  <AlertDescription>
                    Failed to load queue: {(queueErr as Error)?.message ?? 'Unknown error'}
                  </AlertDescription>
                </Alert>
              ) : queueItems.length === 0 ? (
                <div className="p-8 text-center text-zinc-400">
                  No active downloads
                </div>
              ) : (
                queueItems.map((item) => (
                  <QueueItem key={item.id} item={item} managerId={managerId} />
                ))
              )}
            </TabsContent>

            <TabsContent value="stuck" className="space-y-4">
              {stuckLoading ? (
                <div className="h-32 bg-zinc-900 rounded-lg animate-pulse" />
              ) : stuckError ? (
                <Alert variant="destructive">
                  <AlertTriangle className="h-4 w-4" />
                  <AlertDescription>
                    Failed to load stuck items: {(stuckErr as Error)?.message ?? 'Unknown error'}
                  </AlertDescription>
                </Alert>
              ) : stuckItems.length === 0 ? (
                <div className="p-8 text-center text-zinc-400">
                  No stuck items
                </div>
              ) : (
                stuckItems.map((item) => (
                  <QueueItem key={item.id} item={item} managerId={managerId} />
                ))
              )}
            </TabsContent>
          </Tabs>
        )}
      </div>
    </AppShell>
  );
}
