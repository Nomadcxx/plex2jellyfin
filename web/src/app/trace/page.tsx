'use client';

import { useState } from 'react';
import { AppShell } from '@/components/layout/AppShell';
import { useTrace, TraceItem } from '@/hooks/useTrace';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Route,
  Search,
  FileSearch,
  Wand2,
  FolderInput,
  MonitorCheck,
  CheckCircle2,
  XCircle,
  CircleDashed,
} from 'lucide-react';

function fmtTime(iso?: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  return d.toLocaleString(undefined, {
    month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit',
  });
}

function parsedSummary(item: TraceItem): string {
  if (!item.parsed_title) return `via ${item.parse_method || 'unknown'} — no title extracted`;
  let s = `via ${item.parse_method || 'unknown'} → “${item.parsed_title}”`;
  if (item.parsed_year) s += ` (${item.parsed_year})`;
  if (item.parsed_season != null && item.parsed_episode != null) {
    s += ` S${String(item.parsed_season).padStart(2, '0')}E${String(item.parsed_episode).padStart(2, '0')}`;
  }
  return s;
}

function jellyfinSummary(item: TraceItem): { text: string; done: boolean } {
  const jf = item.jellyfin;
  if (jf?.item_id) {
    const ids = [
      jf.imdb_id && `imdb ${jf.imdb_id}`,
      jf.tmdb_id && `tmdb ${jf.tmdb_id}`,
      jf.tvdb_id && `tvdb ${jf.tvdb_id}`,
    ].filter(Boolean).join(', ');
    return {
      text: `matched item ${jf.item_id}${ids ? ` (${ids})` : ''}${jf.resolved_at ? ` at ${fmtTime(jf.resolved_at)}` : ''}`,
      done: true,
    };
  }
  if (jf?.identified === false) return { text: 'seen by Jellyfin but not identified yet', done: false };
  return { text: 'not confirmed by Jellyfin yet', done: false };
}

function Step({
  icon: Icon,
  label,
  text,
  muted,
  time,
}: {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  text: string;
  muted?: boolean;
  time?: string;
}) {
  return (
    <div className="flex items-start gap-3">
      <div className={`mt-0.5 p-1.5 rounded-md border ${muted ? 'border-zinc-800 text-zinc-600' : 'border-zinc-700 text-zinc-300'}`}>
        <Icon className="h-3.5 w-3.5" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline justify-between gap-3">
          <span className="text-xs font-semibold uppercase tracking-wider text-zinc-500">{label}</span>
          {time && <span className="text-xs text-zinc-600 whitespace-nowrap">{time}</span>}
        </div>
        <p className={`text-sm break-all ${muted ? 'text-zinc-500' : 'text-zinc-200'}`}>{text}</p>
      </div>
    </div>
  );
}

function outcomeBadge(item: TraceItem) {
  const outcome = (item.outcome || '').toUpperCase();
  if (outcome === 'SUCCESS') {
    return (
      <span className="flex items-center gap-1.5 text-xs font-medium text-emerald-400 bg-emerald-500/10 border border-emerald-500/20 px-2.5 py-1 rounded-full">
        <CheckCircle2 className="h-3.5 w-3.5" /> Success
      </span>
    );
  }
  if (outcome === 'FAIL' || item.error) {
    return (
      <span className="flex items-center gap-1.5 text-xs font-medium text-rose-400 bg-rose-500/10 border border-rose-500/20 px-2.5 py-1 rounded-full">
        <XCircle className="h-3.5 w-3.5" /> Failed
      </span>
    );
  }
  return (
    <span className="flex items-center gap-1.5 text-xs font-medium text-zinc-400 bg-zinc-800/60 border border-zinc-700/50 px-2.5 py-1 rounded-full">
      <CircleDashed className="h-3.5 w-3.5" /> {item.outcome || 'Pending'}
    </span>
  );
}

function TraceCard({ item }: { item: TraceItem }) {
  const jf = jellyfinSummary(item);
  const sourceDir = item.source_path.endsWith(item.source_filename)
    ? item.source_path.slice(0, -item.source_filename.length).replace(/\/$/, '')
    : item.source_path;

  return (
    <Card>
      <CardContent className="p-4 sm:p-5 space-y-4">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
          <p className="font-mono text-sm text-zinc-100 break-all">{item.source_filename}</p>
          {outcomeBadge(item)}
        </div>
        <div className="space-y-3">
          <Step icon={FileSearch} label="Detected" text={`in ${sourceDir || '(unknown)'}`} time={fmtTime(item.event_at)} />
          <Step icon={Wand2} label="Parsed" text={parsedSummary(item) + (item.stripped_tokens ? ` — stripped: ${item.stripped_tokens}` : '')} />
          <Step
            icon={FolderInput}
            label="Moved"
            text={item.target_path ? `→ ${item.target_path}` : 'not moved'}
            muted={!item.target_path}
            time={fmtTime(item.target_at)}
          />
          <Step icon={MonitorCheck} label="Jellyfin" text={jf.text} muted={!jf.done} />
          {item.error && (
            <Step icon={XCircle} label="Error" text={item.error} />
          )}
        </div>
      </CardContent>
    </Card>
  );
}

export default function TracePage() {
  const [query, setQuery] = useState('');
  const { data, isLoading } = useTrace(query);
  const items = data?.items ?? [];

  return (
    <AppShell>
      <div className="space-y-6 max-w-4xl mx-auto pb-12">
        <div className="flex flex-col md:flex-row md:items-end justify-between gap-4">
          <div>
            <h1 className="text-3xl font-bold flex items-center gap-3">
              <div className="p-2.5 bg-terminal-cyan/10 text-terminal-cyan rounded-md">
                <Route className="h-6 w-6" />
              </div>
              File Trace
            </h1>
            <p className="text-zinc-400 mt-2 max-w-xl">
              What the daemon actually did to each file: detected, parsed,
              moved, and confirmed against Jellyfin.
            </p>
          </div>
        </div>

        <div className="relative">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-zinc-500" />
          <Input
            placeholder="Filter by filename or path…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className="pl-9"
          />
        </div>

        {isLoading ? (
          <div className="space-y-4">
            {[0, 1, 2].map((i) => <Skeleton key={i} className="h-40 w-full rounded-md" />)}
          </div>
        ) : items.length === 0 ? (
          <div className="flex flex-col items-center justify-center p-12 bg-zinc-900/30 rounded-lg border border-zinc-800/50 text-center">
            <h3 className="text-xl font-bold text-zinc-300">
              {query ? 'No files match that filter' : 'No pipeline activity yet'}
            </h3>
            <p className="text-zinc-400 mt-2 max-w-md">
              {query
                ? 'Try a shorter fragment of the filename.'
                : 'Once the daemon organizes a download, its full journey shows up here.'}
            </p>
          </div>
        ) : (
          <div className="space-y-4">
            {items.map((item) => <TraceCard key={item.id} item={item} />)}
          </div>
        )}
      </div>
    </AppShell>
  );
}
