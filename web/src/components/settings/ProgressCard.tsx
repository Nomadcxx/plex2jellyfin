'use client';

import { Progress } from '@/components/ui/progress';

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

export function ProgressCard({ title, events }: Props) {
  const last = events[events.length - 1];
  const pct = last?.total ? Math.round(((last.current ?? 0) / last.total) * 100) : 0;
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
            {last.phase}: {last.msg} ({pct}%)
          </p>
          <Progress value={pct} className="mt-2" />
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
