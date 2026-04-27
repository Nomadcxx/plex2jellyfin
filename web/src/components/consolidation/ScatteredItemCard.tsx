'use client';

import { useState } from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { LocationList } from './LocationList';
import { ConsolidationPreview } from './ConsolidationPreview';
import { 
  FolderTree, 
  MoveRight, 
  Package,
  Calendar,
  PlayCircle,
  CheckCircle,
} from 'lucide-react';
import { components } from '@/types/api';
import { formatBytes } from '@/lib/utils';
import { useConsolidate } from '@/hooks/useConsolidate';
import { toast } from 'sonner';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

type ScatteredItem = components['schemas']['ScatteredItem'];

interface ScatteredItemCardProps {
  item: ScatteredItem;
}

export function ScatteredItemCard({ item }: ScatteredItemCardProps) {
  const [showPreview, setShowPreview] = useState(false);
  const [showConfirm, setShowConfirm] = useState(false);
  const consolidateMutation = useConsolidate(item.id || 0);

  const handleDryRun = async () => {
    try {
      const result = await consolidateMutation.mutateAsync({ dryRun: true });
      toast.success(`Dry run complete: Would move ${result.filesMoved} files`);
      setShowPreview(false);
    } catch (error) {
      toast.error('Dry run failed');
      console.error(error);
    }
  };

  const handleConsolidate = async () => {
    try {
      const result = await consolidateMutation.mutateAsync({ dryRun: false });
      toast.success(
        `Consolidated successfully: ${result.filesMoved} files moved (${formatBytes(result.bytesMoved || 0)})`
      );
      setShowConfirm(false);
    } catch (error) {
      toast.error('Consolidation failed');
      console.error(error);
    }
  };

  return (
    <>
      <Card className="overflow-hidden hover:shadow-lg transition-shadow">
        <CardHeader className="bg-accent/30 border-b border-border">
          <div className="flex items-start justify-between">
            <div className="flex-1">
              <CardTitle className="flex items-center gap-2 text-xl">
                {item.mediaType === 'movie' ? (
                  <Package className="h-5 w-5 text-blue-500" />
                ) : (
                  <PlayCircle className="h-5 w-5 text-purple-500" />
                )}
                {item.title}
                {item.year && (
                  <span className="text-muted-foreground font-normal">({item.year})</span>
                )}
              </CardTitle>
              <div className="flex items-center gap-2 mt-2">
                <Badge variant="outline" className="capitalize">
                  {item.mediaType}
                </Badge>
                <Badge variant="secondary">
                  {item.locations?.length || 0} locations
                </Badge>
              </div>
            </div>
          </div>
        </CardHeader>

        <CardContent className="p-6 space-y-6">
          <div className="grid grid-cols-2 gap-4">
            <div className="bg-accent/50 p-4 rounded-lg border border-border">
              <div className="flex items-center gap-2 text-sm text-muted-foreground mb-1">
                <MoveRight className="h-4 w-4" />
                Files to Move
              </div>
              <div className="text-2xl font-bold">{item.filesToMove || 0}</div>
            </div>

            <div className="bg-accent/50 p-4 rounded-lg border border-border">
              <div className="flex items-center gap-2 text-sm text-muted-foreground mb-1">
                <Package className="h-4 w-4" />
                Total Size
              </div>
              <div className="text-2xl font-bold">
                {formatBytes(item.bytesToMove || 0)}
              </div>
            </div>
          </div>

          <LocationList 
            locations={item.locations || []} 
            targetLocation={item.targetLocation}
          />

          <div className="flex gap-2">
            <Button
              variant="outline"
              onClick={() => setShowPreview(true)}
              className="flex-1"
            >
              <FolderTree className="h-4 w-4 mr-2" />
              Preview
            </Button>
            <Button
              onClick={() => setShowConfirm(true)}
              className="flex-1 bg-green-600 hover:bg-green-700"
            >
              <CheckCircle className="h-4 w-4 mr-2" />
              Consolidate
            </Button>
          </div>
        </CardContent>
      </Card>

      <Dialog open={showPreview} onOpenChange={setShowPreview}>
        <DialogContent className="max-w-3xl">
          <DialogHeader>
            <DialogTitle>Consolidation Preview</DialogTitle>
            <DialogDescription>
              Review what will happen before consolidating {item.title}
            </DialogDescription>
          </DialogHeader>

          <ConsolidationPreview item={item} />

          <DialogFooter>
            <Button variant="ghost" onClick={() => setShowPreview(false)}>
              Close
            </Button>
            <Button
              variant="outline"
              onClick={handleDryRun}
              disabled={consolidateMutation.isPending}
            >
              {consolidateMutation.isPending ? 'Running...' : 'Dry Run'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={showConfirm} onOpenChange={setShowConfirm}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Confirm Consolidation</DialogTitle>
            <DialogDescription>
              This will move {item.filesToMove} files to the target location. This action cannot be undone.
            </DialogDescription>
          </DialogHeader>

          <div className="py-4">
            <div className="space-y-2 p-4 bg-yellow-500/10 border border-yellow-500/30 rounded-lg">
              <div className="flex items-start gap-2">
                <MoveRight className="h-5 w-5 text-yellow-400 shrink-0 mt-0.5" />
                <div className="space-y-1">
                  <p className="text-sm font-semibold text-yellow-400">
                    {item.filesToMove} files will be moved
                  </p>
                  <p className="text-xs text-muted-foreground">
                    Total size: {formatBytes(item.bytesToMove || 0)}
                  </p>
                  <p className="text-xs text-muted-foreground mt-2">
                    Target: <code className="bg-black/30 px-1 py-0.5 rounded">{item.targetLocation}</code>
                  </p>
                </div>
              </div>
            </div>
          </div>

          <DialogFooter>
            <Button
              variant="ghost"
              onClick={() => setShowConfirm(false)}
            >
              Cancel
            </Button>
            <Button
              onClick={handleConsolidate}
              disabled={consolidateMutation.isPending}
              className="bg-green-600 hover:bg-green-700"
            >
              {consolidateMutation.isPending ? 'Consolidating...' : 'Consolidate'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
