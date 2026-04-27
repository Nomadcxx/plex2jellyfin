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
  const [historyLimit, setHistoryLimit] = useState(100);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const [hasMore, setHasMore] = useState(true);
  const attemptRef = useRef(0);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const sourceRef = useRef<EventSource | null>(null);
  const closedRef = useRef(false);
  const eventCapRef = useRef(100);

  // Backend caps `limit` at 500 and does not support offset, so paging works
  // by re-fetching with a larger limit and merging by id. Once we hit the cap
  // we stop offering "Load older".
  const HARD_LIMIT = 500;

  const fetchHistory = async (limit: number) => {
    const res = await fetch(`/api/v1/activity?limit=${limit}`);
    if (!res.ok) throw new Error(`status ${res.status}`);
    const body = (await res.json()) as { events?: ActivityEvent[] };
    return Array.isArray(body.events) ? body.events : [];
  };

  const mergeOlder = (incoming: ActivityEvent[]) => {
    setEvents((prev) => {
      const seen = new Set(prev.map((e) => e.id));
      const merged = [...prev];
      for (const e of incoming) {
        if (!seen.has(e.id)) merged.push(e);
      }
      return merged.slice(0, eventCapRef.current);
    });
  };

  const loadOlder = async () => {
    if (isLoadingMore || !hasMore) return;
    const next = Math.min(historyLimit + 100, HARD_LIMIT);
    if (next === historyLimit) {
      setHasMore(false);
      return;
    }
    setIsLoadingMore(true);
    try {
      const list = await fetchHistory(next);
      eventCapRef.current = next;
      mergeOlder(list);
      setHistoryLimit(next);
      // If the server returned fewer than next entries we've exhausted history.
      if (list.length < next) setHasMore(false);
      if (next >= HARD_LIMIT) setHasMore(false);
    } catch {
      // surface failures via reconnection state; no toast here to avoid noise
    } finally {
      setIsLoadingMore(false);
    }
  };

  useEffect(() => {
    closedRef.current = false;

    // Seed with recent history so the page isn't empty until the next live
    // event arrives. SSE only sends new entries from this point forward.
    const seed = async () => {
      try {
        const list = await fetchHistory(100);
        if (closedRef.current) return;
        if (list.length > 0) mergeOlder(list);
      } catch {
        // network errors are surfaced via the SSE connection state instead
      }
    };
    seed();

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
          setEvents((prev) => {
            if (prev.some((e) => e.id === data.id)) return prev;
            return [data, ...prev].slice(0, eventCapRef.current);
          });
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

  return { events, isConnected, loadOlder, isLoadingMore, hasMore };
}
