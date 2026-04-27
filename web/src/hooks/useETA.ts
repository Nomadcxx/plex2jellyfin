'use client';

import { useMemo, useRef } from 'react';
import type { OpEvent } from '@/components/settings/ProgressCard';

export type ETAResult = {
  ratePerSec: number;
  etaSeconds: number;
  etaFormatted: string;
  rateFormatted: string;
  reliable: boolean;
  pct: number;
};

type Sample = { t: number; current: number };

function fmtDuration(s: number): string {
  if (!isFinite(s) || s < 0) return '—';
  if (s < 1) return '<1s';
  if (s < 60) return `${Math.round(s)}s`;
  const m = Math.floor(s / 60);
  const r = Math.round(s % 60);
  if (m < 60) return `${m}m ${r}s`;
  const h = Math.floor(m / 60);
  const rm = m % 60;
  return `${h}h ${rm}m`;
}

function fmtRate(r: number): string {
  if (!isFinite(r) || r <= 0) return '—';
  if (r < 1) return `${r.toFixed(2)} /s`;
  if (r < 100) return `${r.toFixed(1)} /s`;
  return `${Math.round(r)} /s`;
}

// useETA derives throughput and ETA from a stream of OpEvents. Samples
// are kept in a sliding window so rate reflects recent activity; it
// stabilises after ~3 samples spanning >=1s so very-fast bursts at
// startup don't produce wild estimates.
export function useETA(events: OpEvent[]): ETAResult {
  const samplesRef = useRef<Sample[]>([]);

  return useMemo(() => {
    const last = events[events.length - 1];
    const current = last?.current ?? 0;
    const total = last?.total ?? 0;
    const pct = total > 0 ? Math.min(100, Math.round((current / total) * 100)) : 0;

    const now = Date.now();
    const samples = samplesRef.current;
    const lastSample = samples[samples.length - 1];
    if (last?.type === 'progress' && (!lastSample || lastSample.current !== current)) {
      samples.push({ t: now, current });
      const cutoff = now - 30_000;
      while (samples.length > 2 && samples[0].t < cutoff) samples.shift();
    }
    if (last && (last.type === 'done' || last.type === 'error' || last.type === 'cancelled')) {
      samplesRef.current = [];
    }

    if (samples.length < 3) {
      return {
        ratePerSec: 0,
        etaSeconds: 0,
        etaFormatted: '—',
        rateFormatted: '—',
        reliable: false,
        pct,
      };
    }
    const first = samples[0];
    const newest = samples[samples.length - 1];
    const dt = (newest.t - first.t) / 1000;
    const dn = newest.current - first.current;
    const rate = dt > 0 ? dn / dt : 0;
    const remaining = total > 0 ? Math.max(0, total - current) : 0;
    const eta = rate > 0 ? remaining / rate : 0;

    return {
      ratePerSec: rate,
      etaSeconds: eta,
      etaFormatted: total > 0 ? fmtDuration(eta) : '—',
      rateFormatted: fmtRate(rate),
      reliable: dt >= 1,
      pct,
    };
  }, [events]);
}
