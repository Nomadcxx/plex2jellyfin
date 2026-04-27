'use client';

import { useEffect, useRef, useState } from 'react';
import { useActivityStream } from '@/hooks/useSSE';
import { ActivityItem } from './ActivityItem';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Pause, Play, ArrowDown } from 'lucide-react';
import { components } from '@/types/api';

type ActivityEvent = components['schemas']['ActivityEvent'];

interface ActivityStreamProps {
  searchQuery?: string;
  filterType?: string;
}

export function ActivityStream({ searchQuery = '', filterType = 'all' }: ActivityStreamProps) {
  const { events, isConnected } = useActivityStream();
  const [isPaused, setIsPaused] = useState(false);
  const [autoScroll, setAutoScroll] = useState(true);
  const [displayedEvents, setDisplayedEvents] = useState<ActivityEvent[]>([]);
  const scrollRef = useRef<HTMLDivElement>(null);
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!isPaused) {
      setDisplayedEvents(events);
    }
  }, [events, isPaused]);

  useEffect(() => {
    if (autoScroll && bottomRef.current) {
      bottomRef.current.scrollIntoView({ behavior: 'smooth' });
    }
  }, [displayedEvents, autoScroll]);

  const filteredEvents = displayedEvents.filter((event) => {
    const matchesType = filterType === 'all' || event.type === filterType;
    const matchesSearch = 
      searchQuery === '' || 
      event.message?.toLowerCase().includes(searchQuery.toLowerCase());
    return matchesType && matchesSearch;
  });

  const handleTogglePause = () => {
    if (isPaused) {
      setDisplayedEvents(events);
    }
    setIsPaused(!isPaused);
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Badge 
            variant={isConnected ? 'default' : 'destructive'}
            className={isConnected ? 'bg-green-600' : ''}
          >
            {isConnected ? 'Live' : 'Disconnected'}
          </Badge>
          
          {isPaused && (
            <Badge variant="outline" className="text-yellow-400 border-yellow-500">
              Paused ({events.length - displayedEvents.length} new)
            </Badge>
          )}
        </div>

        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={handleTogglePause}
          >
            {isPaused ? (
              <Play className="h-4 w-4 mr-2" />
            ) : (
              <Pause className="h-4 w-4 mr-2" />
            )}
            {isPaused ? 'Resume' : 'Pause'}
          </Button>

          <Button
            variant="outline"
            size="sm"
            onClick={() => setAutoScroll(!autoScroll)}
            className={autoScroll ? 'bg-accent' : ''}
          >
            <ArrowDown className="h-4 w-4 mr-2" />
            Auto-scroll
          </Button>
        </div>
      </div>

      <ScrollArea className="h-[600px] rounded-lg border bg-card" ref={scrollRef}>
        <div className="p-4 space-y-3">
          {filteredEvents.length > 0 ? (
            <>
              {filteredEvents.map((event, index) => (
                <ActivityItem key={`${event.id}-${index}`} event={event} />
              ))}
              <div ref={bottomRef} />
            </>
          ) : (
            <div className="flex flex-col items-center justify-center py-12 text-center">
              <p className="text-muted-foreground">No activity yet</p>
              {searchQuery && (
                <p className="text-sm text-muted-foreground mt-2">
                  Try adjusting your search or filter
                </p>
              )}
            </div>
          )}
        </div>
      </ScrollArea>
    </div>
  );
}
