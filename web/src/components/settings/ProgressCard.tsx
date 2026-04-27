'use client';

import { Progress } from '@/components/ui/progress';
import { useETA } from '@/hooks/useETA';

export type OpEvent = {
  type: 'progress' | 'done' | 'error' | 'cancelled';
  phase?: string;
  msg?: string;
  current?: number;
  total?: number;
  code?: string;
};

type Props = {
  title: string;
  events: OpEvent[];
};

function fmtNum(n: number | undefined): string {
  if (n === undefined) return '0';
  return new Intl.NumberFormat().format(n);
}

export function ProgressCard({ title, events }: Props) {
  const last = events[events.length - 1];
  const eta = useETA(events);
  const pct = eta.pct;
  const isDone = last?.type === 'done';
  const isError = last?.type === 'error';
  const isCancelled = last?.type === 'cancelled';

  return (
    <div className="rounded border p-4">
      <h3 className="font-semibold">{title}</h3>
      {isDone && <p className="text-green-600">Complete.</p>}
      {isError && <p className="text-destructive">Failed: {last?.msg}</p>}
      {isCancelled && <p>Cancelled.</p>}
      {!isDone && !isError && !isCancelled && last?.type === 'progress' && (
        <>
          <p className="text-sm text-muted-foreground">
            <span className="font-medium text-zinc-200">{last.phase}</span>
            {last.msg ? <>: <span className="break-all">{last.msg}</span></> : null}
          </p>
          <Progress value={pct} className="mt-2" />
          <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground tabular-nums">
            <span>
              {fmtNum(last.current)}
              {last.total ? <> / {fmtNum(last.total)}</> : null}
              {last.total ? <> ({pct}%)</> : null}
            </span>
            {eta.reliable && (
              <>
                <span>· {eta.rateFormatted}</span>
                {last.total ? <span>· ETA {eta.etaFormatted}</span> : null}
              </>
            )}
          </div>
        </>
      )}
      <ul className="mt-3 max-h-48 overflow-y-auto text-xs font-mono text-muted-foreground">
        {events.slice(-30).map((e, i) => (
          <li key={i}>
            [{e.type}] {e.msg}
          </li>
        ))}
      </ul>
    </div>
  );
}
