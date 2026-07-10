'use client';

import { useState } from 'react';
import { components } from '@/types/api';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { 
  FilePlus, 
  FileMinus, 
  RefreshCw, 
  Database, 
  Bot, 
  AlertTriangle,
  FileCheck,
  ChevronDown,
  ChevronRight,
} from 'lucide-react';
import { formatDistanceToNow } from 'date-fns';

type ActivityEvent = components['schemas']['ActivityEvent'];

interface ActivityItemProps {
  event: ActivityEvent;
}

const iconMap: Record<string, any> = {
  FILE_DETECTED: FilePlus,
  FILE_ORGANIZED: FileCheck,
  FILE_DELETED: FileMinus,
  DUPLICATE_FOUND: Database,
  SYNC_COMPLETED: RefreshCw,
  ERROR: AlertTriangle,
  AI_AUDIT: Bot,
};

const severityColors: Record<string, string> = {
  FILE_DETECTED: 'bg-blue-500/10 text-blue-400 border-blue-500/30',
  FILE_ORGANIZED: 'bg-green-500/10 text-green-400 border-green-500/30',
  FILE_DELETED: 'bg-red-500/10 text-red-400 border-red-500/30',
  DUPLICATE_FOUND: 'bg-yellow-500/10 text-yellow-400 border-yellow-500/30',
  SYNC_COMPLETED: 'bg-terminal-cyan/10 text-terminal-cyan border-terminal-cyan/30',
  ERROR: 'bg-red-500/10 text-red-400 border-red-500/30',
  AI_AUDIT: 'bg-cyan-500/10 text-cyan-400 border-cyan-500/30',
};

export function ActivityItem({ event }: ActivityItemProps) {
  const [expanded, setExpanded] = useState(false);
  const Icon = iconMap[event.type || 'FILE_DETECTED'] || FilePlus;
  const colorClass = severityColors[event.type || 'FILE_DETECTED'] || severityColors.FILE_DETECTED;

  const timeAgo = event.timestamp 
    ? formatDistanceToNow(new Date(event.timestamp), { addSuffix: true })
    : 'unknown time';

  return (
    <div className={`p-4 rounded-lg border ${colorClass} transition-all hover:shadow-md`}>
      <div className="flex items-start gap-3">
        <div className="mt-1 shrink-0">
          <Icon className="h-5 w-5" />
        </div>
        
        <div className="flex-1 min-w-0">
          <div className="flex items-start justify-between gap-2 mb-1">
            <p className="text-sm font-medium break-words flex-1">
              {event.message}
            </p>
            <Badge 
              variant="outline" 
              className="shrink-0 text-xs"
            >
              {event.type?.replace(/_/g, ' ')}
            </Badge>
          </div>
          
          <p className="text-xs text-muted-foreground">
            {timeAgo}
          </p>

          {event.id && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setExpanded(!expanded)}
              className="mt-2 h-6 px-2 text-xs"
            >
              {expanded ? (
                <ChevronDown className="h-3 w-3 mr-1" />
              ) : (
                <ChevronRight className="h-3 w-3 mr-1" />
              )}
              {expanded ? 'Hide' : 'Show'} Details
            </Button>
          )}

          {expanded && (
            <div className="mt-2 pt-2 border-t border-border/50">
              <div className="text-xs font-mono bg-black/30 p-2 rounded overflow-x-auto">
                <div>ID: {event.id}</div>
                <div>Type: {event.type}</div>
                <div>Timestamp: {event.timestamp}</div>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
