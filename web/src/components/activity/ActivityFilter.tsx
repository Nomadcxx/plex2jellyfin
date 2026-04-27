'use client';

import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Search, X } from 'lucide-react';

export type ActivityFilterType = 
  | 'all'
  | 'FILE_DETECTED'
  | 'FILE_ORGANIZED'
  | 'FILE_DELETED'
  | 'DUPLICATE_FOUND'
  | 'SYNC_COMPLETED'
  | 'ERROR'
  | 'AI_AUDIT';

interface ActivityFilterProps {
  activeFilter: ActivityFilterType;
  onFilterChange: (filter: ActivityFilterType) => void;
  searchQuery: string;
  onSearchChange: (query: string) => void;
  counts: Record<string, number>;
}

const filterLabels: Record<ActivityFilterType, string> = {
  all: 'All',
  FILE_DETECTED: 'Detected',
  FILE_ORGANIZED: 'Organized',
  FILE_DELETED: 'Deleted',
  DUPLICATE_FOUND: 'Duplicates',
  SYNC_COMPLETED: 'Sync',
  ERROR: 'Errors',
  AI_AUDIT: 'AI Audit',
};

export function ActivityFilter({
  activeFilter,
  onFilterChange,
  searchQuery,
  onSearchChange,
  counts,
}: ActivityFilterProps) {
  const filters: ActivityFilterType[] = [
    'all',
    'FILE_DETECTED',
    'FILE_ORGANIZED',
    'FILE_DELETED',
    'DUPLICATE_FOUND',
    'SYNC_COMPLETED',
    'ERROR',
    'AI_AUDIT',
  ];

  return (
    <div className="space-y-4">
      <div className="relative">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
        <Input
          placeholder="Search activity..."
          value={searchQuery}
          onChange={(e: React.ChangeEvent<HTMLInputElement>) => onSearchChange(e.target.value)}
          className="pl-9 pr-9"
        />
        {searchQuery && (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => onSearchChange('')}
            className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7 p-0"
          >
            <X className="h-4 w-4" />
          </Button>
        )}
      </div>

      <div className="flex flex-wrap gap-2">
        {filters.map((filter) => {
          const count = counts[filter] || 0;
          const isActive = activeFilter === filter;
          
          return (
            <Button
              key={filter}
              variant={isActive ? 'default' : 'outline'}
              size="sm"
              onClick={() => onFilterChange(filter)}
              className={isActive ? 'shadow-md' : ''}
            >
              {filterLabels[filter]}
              {count > 0 && (
                <Badge 
                  variant="secondary" 
                  className="ml-2 px-1.5 py-0 text-xs"
                >
                  {count}
                </Badge>
              )}
            </Button>
          );
        })}
      </div>
    </div>
  );
}
