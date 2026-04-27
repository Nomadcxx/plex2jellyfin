import { useEffect, useRef, useState } from 'react';
import { components } from '@/types/api';

type ActivityEvent = components['schemas']['ActivityEvent'];

// useActivityStream connects to /api/v1/activity/stream with exponential
// backoff reconnection. Without reconnect the UI silently freezes the moment
// the daemon restarts or the network blips — the old implementation gave up
// after the first onerror.
export function useActivityStream() {
  const [events, setEvents] = useState<ActivityEvent[]>([]);
  const [isConnected, setIsConnected] = useState(false);
  const attemptRef = useRef(0);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const sourceRef = useRef<EventSource | null>(null);
  const closedRef = useRef(false);

  useEffect(() => {
    closedRef.current = false;

    const connect = () => {
      if (closedRef.current) return;
      const eventSource = new EventSource('/api/v1/activity/stream');
      sourceRef.current = eventSource;

      eventSource.onopen = () => {
        setIsConnected(true);
        attemptRef.current = 0;
      };

      const handleEvent = (event: MessageEvent) => {
        try {
          const data = JSON.parse(event.data) as ActivityEvent;
          setEvents((prev) => [data, ...prev].slice(0, 100));
        } catch (error) {
          console.error('Failed to parse SSE data:', error);
        }
      };

      // The server emits live entries as named "activity" events; default
      // onmessage only fires for unnamed events, so register both so a
      // future change to unnamed events doesn't silently break the UI.
      eventSource.addEventListener('activity', handleEvent as EventListener);
      eventSource.onmessage = handleEvent;

      eventSource.onerror = () => {
        setIsConnected(false);
        eventSource.close();
        if (closedRef.current) return;
        // Exponential backoff capped at 30s; first retry is immediate-ish (1s).
        const delayMs = Math.min(30_000, 1000 * 2 ** attemptRef.current);
        attemptRef.current += 1;
        timerRef.current = setTimeout(connect, delayMs);
      };
    };

    connect();

    return () => {
      closedRef.current = true;
      if (timerRef.current) clearTimeout(timerRef.current);
      sourceRef.current?.close();
      setIsConnected(false);
    };
  }, []);

  return { events, isConnected };
}
