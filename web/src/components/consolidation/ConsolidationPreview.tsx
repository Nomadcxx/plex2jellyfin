'use client';

import { components } from '@/types/api';
import { ArrowRight, HardDrive, FolderTree, AlertCircle } from 'lucide-react';
import { formatBytes } from '@/lib/utils';
import { Separator } from '@/components/ui/separator';

type ScatteredItem = components['schemas']['ScatteredItem'];

interface ConsolidationPreviewProps {
  item: ScatteredItem;
}

export function ConsolidationPreview({ item }: ConsolidationPreviewProps) {
  const nonTargetLocations = item.locations?.filter(loc => loc !== item.targetLocation) || [];
  
  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-6">
        <div>
          <div className="flex items-center gap-2 mb-3">
            <FolderTree className="h-4 w-4 text-red-400" />
            <h3 className="font-semibold text-sm">Current State</h3>
          </div>
          <div className="space-y-2">
            {item.locations?.map((location, index) => (
              <div
                key={index}
                className="p-3 bg-red-500/10 border border-red-500/30 rounded-lg"
              >
                <code className="text-xs break-all">{location}</code>
              </div>
            ))}
          </div>
          <div className="mt-3 text-xs text-muted-foreground">
            Scattered across {item.locations?.length || 0} locations
          </div>
        </div>

        <div>
          <div className="flex items-center gap-2 mb-3">
            <HardDrive className="h-4 w-4 text-green-400" />
            <h3 className="font-semibold text-sm">After Consolidation</h3>
          </div>
          <div className="p-3 bg-green-500/10 border border-green-500/30 rounded-lg">
            <code className="text-xs break-all">{item.targetLocation}</code>
          </div>
          <div className="mt-3 text-xs text-muted-foreground">
            All files in one location
          </div>
        </div>
      </div>

      <Separator />

      <div className="space-y-4">
        <h3 className="font-semibold text-sm flex items-center gap-2">
          <ArrowRight className="h-4 w-4" />
          What will happen
        </h3>

        <div className="space-y-3">
          <div className="flex items-start gap-3 p-3 bg-accent/50 rounded-lg border border-border">
            <div className="shrink-0 w-6 h-6 rounded-full bg-blue-500/20 text-blue-400 flex items-center justify-center text-xs font-bold">
              1
            </div>
            <div>
              <p className="text-sm font-medium">Move files from scattered locations</p>
              <p className="text-xs text-muted-foreground mt-1">
                {item.filesToMove} files ({formatBytes(item.bytesToMove || 0)}) will be moved from:
              </p>
              <div className="mt-2 space-y-1">
                {nonTargetLocations.map((loc, index) => (
                  <code key={index} className="text-xs bg-black/30 px-2 py-1 rounded block">
                    {loc}
                  </code>
                ))}
              </div>
            </div>
          </div>

          <div className="flex items-start gap-3 p-3 bg-accent/50 rounded-lg border border-border">
            <div className="shrink-0 w-6 h-6 rounded-full bg-green-500/20 text-green-400 flex items-center justify-center text-xs font-bold">
              2
            </div>
            <div>
              <p className="text-sm font-medium">Consolidate to primary location</p>
              <p className="text-xs text-muted-foreground mt-1">
                All files will be in:
              </p>
              <code className="text-xs bg-black/30 px-2 py-1 rounded block mt-2">
                {item.targetLocation}
              </code>
            </div>
          </div>

          <div className="flex items-start gap-3 p-3 bg-accent/50 rounded-lg border border-border">
            <div className="shrink-0 w-6 h-6 rounded-full bg-purple-500/20 text-purple-400 flex items-center justify-center text-xs font-bold">
              3
            </div>
            <div>
              <p className="text-sm font-medium">Clean up empty directories</p>
              <p className="text-xs text-muted-foreground mt-1">
                Empty directories will be removed from old locations
              </p>
            </div>
          </div>
        </div>
      </div>

      <div className="p-4 bg-yellow-500/10 border border-yellow-500/30 rounded-lg flex items-start gap-3">
        <AlertCircle className="h-5 w-5 text-yellow-400 shrink-0" />
        <div>
          <p className="text-sm font-semibold text-yellow-400">Storage Requirements</p>
          <p className="text-xs text-muted-foreground mt-1">
            Ensure the target location has at least {formatBytes(item.bytesToMove || 0)} of free space
          </p>
        </div>
      </div>
    </div>
  );
}
