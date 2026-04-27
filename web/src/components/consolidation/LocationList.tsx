'use client';

import { Badge } from '@/components/ui/badge';
import { HardDrive, FolderTree } from 'lucide-react';
import { components } from '@/types/api';

type ScatteredItem = components['schemas']['ScatteredItem'];

interface LocationListProps {
  locations: string[];
  targetLocation?: string;
}

export function LocationList({ locations, targetLocation }: LocationListProps) {
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2 text-sm text-muted-foreground mb-3">
        <FolderTree className="h-4 w-4" />
        <span className="font-medium">Scattered across {locations.length} locations</span>
      </div>

      {locations.map((location, index) => {
        const isTarget = location === targetLocation;
        
        return (
          <div
            key={index}
            className={`flex items-center gap-3 p-3 rounded-lg border transition-colors ${
              isTarget
                ? 'bg-green-500/10 border-green-500/30'
                : 'bg-accent/50 border-border'
            }`}
          >
            <HardDrive className={`h-4 w-4 shrink-0 ${isTarget ? 'text-green-400' : 'text-muted-foreground'}`} />
            <code className="text-xs flex-1 break-all">
              {location}
            </code>
            {isTarget && (
              <Badge variant="outline" className="text-green-400 border-green-500 shrink-0">
                Target
              </Badge>
            )}
          </div>
        );
      })}
    </div>
  );
}
