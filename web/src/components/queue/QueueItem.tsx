'use client';

import { useState } from 'react';
import { toast } from 'sonner';
import { formatBytes } from '@/lib/utils';
import { Download, X, AlertTriangle } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { AlertDialog } from '@/components/ui/alert-dialog';
import { useClearQueueItem } from '@/hooks/useDashboard';

type QueueItemData = {
  id?: number;
  title?: string;
  status?: string;
  size?: number;
  sizeRemaining?: number;
  timeLeft?: string;
  downloadClient?: string;
  errorMessage?: string;
  isStuck?: boolean;
  progress?: number;
};

interface QueueItemProps {
  item: QueueItemData;
  managerId: string;
}

export function QueueItem({ item, managerId }: QueueItemProps) {
  const clearMutation = useClearQueueItem(managerId);
  const [confirmOpen, setConfirmOpen] = useState(false);

  const handleConfirm = () => {
    clearMutation.mutate(
      { itemId: item.id || 0 },
      {
        onSuccess: () => toast.success(`Removed "${item.title}" from queue`),
        onError: (err: unknown) =>
          toast.error(
            `Failed to remove: ${(err as Error)?.message ?? 'Unknown error'}`,
          ),
      },
    );
  };

  const progress = item.progress || 
    (item.size && item.sizeRemaining !== undefined
      ? Math.round(((item.size - item.sizeRemaining) / item.size) * 100)
      : 0);

  return (
    <div className="vision-card-secondary p-4">
      <div className="flex items-start justify-between gap-4">
        <div className="flex-1 min-w-0">
          <h4 className="font-medium truncate" title={item.title}>
            {item.title}
          </h4>
          
          <div className="flex items-center gap-2 mt-1 text-sm text-zinc-400">
            <Download className="h-3 w-3" />
            <span>{formatBytes(item.size || 0)}</span>
            {item.downloadClient && (
              <>
                <span>·</span>
                <span>{item.downloadClient}</span>
              </>
            )}
          </div>

          {item.errorMessage && (
            <div className="flex items-center gap-2 mt-2 text-sm text-red-400">
              <AlertTriangle className="h-3 w-3" />
              <span className="truncate">{item.errorMessage}</span>
            </div>
          )}

          {progress > 0 && progress < 100 && (
            <div className="mt-3">
              <div className="h-2 bg-zinc-800 rounded-full overflow-hidden">
                <div
                  className="h-full bg-blue-500 transition-all duration-300"
                  style={{ width: `${progress}%` }}
                />
              </div>
              <div className="flex justify-between mt-1 text-xs text-zinc-500">
                <span>{progress}%</span>
                <span>{item.timeLeft || 'calculating...'}</span>
              </div>
            </div>
          )}
        </div>

        <Button
          variant="ghost"
          size="sm"
          aria-label="Remove from queue"
          className="text-zinc-400 hover:text-red-400 flex-shrink-0"
          onClick={() => setConfirmOpen(true)}
          disabled={clearMutation.isPending}
        >
          <X className="h-4 w-4" />
        </Button>
      </div>
      <AlertDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title="Remove from queue?"
        description={item.title ? `"${item.title}" will be removed from the download client.` : undefined}
        confirmLabel="Remove"
        destructive
        onConfirm={handleConfirm}
      />
    </div>
  );
}
