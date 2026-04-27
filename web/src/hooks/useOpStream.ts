'use client';

import { useEffect, useState } from 'react';
import type { OpEvent } from '@/components/settings/ProgressCard';

const STORAGE_KEY = 'jellywatch.activeOp';

export function useOpStream(opID: string | null) {
  const [events, setEvents] = useState<OpEvent[]>([]);

  useEffect(() => {
    if (!opID) return;
    sessionStorage.setItem(STORAGE_KEY, opID);
    const es = new EventSource(`/api/events/op/${opID}`);
    es.onmessage = (ev) => {
      try {
        const f = JSON.parse(ev.data);
        setEvents((prev) => [...prev, f]);
        if (f.type === 'done' || f.type === 'error' || f.type === 'cancelled') {
          es.close();
          sessionStorage.removeItem(STORAGE_KEY);
        }
      } catch {
        // ignore malformed frames
      }
    };
    return () => es.close();
  }, [opID]);

  return { events };
}

export function reattachActiveOp(): string | null {
  if (typeof window === 'undefined') return null;
  return sessionStorage.getItem(STORAGE_KEY);
}
