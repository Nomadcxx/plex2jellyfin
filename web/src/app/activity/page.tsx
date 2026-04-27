'use client';

import Image from 'next/image';
import { AppShell } from '@/components/layout/AppShell';
import { useActivity } from '@/hooks/useActivity';
import { Activity, Loader2, AlertTriangle, FileText, CheckCircle2, Info, FolderSync, ArrowRight, Download } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';

// Simple relative time formatter
function timeAgo(date: Date | string | undefined): string {
  if (!date) return 'Unknown time';
  const d = new Date(date);
  const now = new Date();
  const diffInSeconds = Math.floor((now.getTime() - d.getTime()) / 1000);

  if (diffInSeconds < 60) return 'Just now';
  
  const diffInMinutes = Math.floor(diffInSeconds / 60);
  if (diffInMinutes < 60) return `${diffInMinutes}m ago`;
  
  const diffInHours = Math.floor(diffInMinutes / 60);
  if (diffInHours < 24) return `${diffInHours}h ago`;
  
  const diffInDays = Math.floor(diffInHours / 24);
  if (diffInDays < 7) return `${diffInDays}d ago`;
  
  return d.toLocaleDateString();
}

function getEventIcon(type: string) {
  const t = type?.toLowerCase() || '';
  if (t.includes('error') || t.includes('failed')) return <AlertTriangle className="h-5 w-5 text-rose-500" />;
  if (t.includes('success') || t.includes('complete')) return <CheckCircle2 className="h-5 w-5 text-emerald-500" />;
  if (t.includes('move') || t.includes('consolidate')) return <FolderSync className="h-5 w-5 text-sky-500" />;
  if (t.includes('download')) return <Download className="h-5 w-5 text-blue-500" />;
  if (t.includes('warning')) return <AlertTriangle className="h-5 w-5 text-amber-500" />;
  return <Info className="h-5 w-5 text-zinc-400" />;
}

function getEventColor(type: string) {
  const t = type?.toLowerCase() || '';
  if (t.includes('error') || t.includes('failed')) return 'bg-rose-500/10 border-rose-500/20';
  if (t.includes('success') || t.includes('complete')) return 'bg-emerald-500/10 border-emerald-500/20';
  if (t.includes('move') || t.includes('consolidate')) return 'bg-sky-500/10 border-sky-500/20';
  if (t.includes('download')) return 'bg-blue-500/10 border-blue-500/20';
  if (t.includes('warning')) return 'bg-amber-500/10 border-amber-500/20';
  return 'bg-zinc-800/50 border-zinc-700/50';
}

export default function ActivityPage() {
  const { data, isLoading, error } = useActivity();

  if (isLoading) {
    return (
      <AppShell>
        <div className="space-y-8 max-w-4xl mx-auto">
          <div className="flex items-center gap-3">
            <div className="h-10 w-10 bg-zinc-900 rounded-lg animate-pulse" />
            <div className="h-8 w-48 bg-zinc-900 rounded animate-pulse" />
          </div>
          <div className="space-y-4 relative pl-8">
            <div className="absolute left-[15px] top-0 bottom-0 w-px bg-zinc-800" />
            {[1, 2, 3, 4].map((i) => (
              <div key={i} className="h-24 bg-zinc-900/50 rounded-xl animate-pulse ml-4" />
            ))}
          </div>
        </div>
      </AppShell>
    );
  }

  if (error) {
    return (
      <AppShell>
        <div className="p-6 bg-rose-500/10 border border-rose-500/20 rounded-xl max-w-4xl mx-auto flex gap-4">
          <AlertTriangle className="h-6 w-6 text-rose-500 flex-shrink-0" />
          <div>
            <h3 className="text-lg font-medium text-rose-400">Failed to load activity feed</h3>
            <p className="text-zinc-400 mt-1">There was an error communicating with the activity stream. Ensure the daemon is running.</p>
          </div>
        </div>
      </AppShell>
    );
  }

  const events = data?.events || [];

  return (
    <AppShell>
      <div className="space-y-8 max-w-4xl mx-auto pb-12">
        {/* Header Section */}
        <div className="flex flex-col md:flex-row md:items-end justify-between gap-6">
          <div>
            <h1 className="text-3xl font-bold flex items-center gap-3">
              <div className="p-2.5 bg-fuchsia-500/10 text-fuchsia-400 rounded-xl">
                <Activity className="h-6 w-6" />
              </div>
              Activity Feed
            </h1>
            <p className="text-zinc-400 mt-2 max-w-xl">
              Real-time events from the Jellywatch daemon, including scans, organizations, and errors.
            </p>
          </div>
          
          <div className="flex items-center gap-3 bg-zinc-900/50 px-4 py-2 rounded-full border border-zinc-800">
            <span className="relative flex h-3 w-3">
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
              <span className="relative inline-flex rounded-full h-3 w-3 bg-emerald-500"></span>
            </span>
            <span className="text-sm font-medium text-zinc-300">Live Connection</span>
          </div>
        </div>

        {/* Content Section */}
        {events.length === 0 ? (
          <div className="flex flex-col items-center justify-center p-12 bg-zinc-900/30 rounded-2xl border border-zinc-800/50 text-center min-h-[400px]">
            <div className="h-48 w-48 mb-6 relative">
              <Image
                src="/illustrations/empty-activity.svg"
                alt="No activity"
                fill
                className="object-contain"
              />
            </div>
            <h3 className="text-2xl font-bold text-zinc-300">Quiet in here...</h3>
            <p className="text-zinc-400 mt-3 max-w-md mx-auto">
              No recent activity has been recorded. Events will appear here automatically when actions occur.
            </p>
          </div>
        ) : (
          <div className="relative pt-4">
            {/* Timeline Line */}
            <div className="absolute left-[27px] top-6 bottom-4 w-px bg-gradient-to-b from-zinc-700 via-zinc-800 to-transparent" />
            
            <div className="space-y-6 relative">
              {events.map((event, idx) => (
                <div key={event.id || idx} className="flex gap-6 relative group">
                  {/* Timeline Node */}
                  <div className="relative mt-1">
                    <div className="w-14 h-14 bg-zinc-950 flex items-center justify-center relative z-10">
                      <div className={`p-2.5 rounded-full border ${getEventColor(event.type || '')} bg-zinc-950 shadow-sm transition-transform group-hover:scale-110`}>
                        {getEventIcon(event.type || '')}
                      </div>
                    </div>
                  </div>
                  
                  {/* Event Card */}
                  <div className="flex-1 pt-2">
                    <Card className="overflow-hidden border-zinc-800/60 bg-zinc-950/50 hover:bg-zinc-900/50 hover:border-zinc-700/80 transition-all duration-300 shadow-sm">
                      <CardContent className="p-4 sm:p-5">
                        <div className="flex flex-col sm:flex-row sm:items-start justify-between gap-4">
                          <div className="space-y-1">
                            <div className="flex items-center gap-2">
                              <span className="text-xs font-semibold uppercase tracking-wider text-zinc-500">
                                {event.type}
                              </span>
                            </div>
                            <p className="text-zinc-200 text-base leading-relaxed break-words">
                              {event.message}
                            </p>
                          </div>
                          
                          <div className="flex items-center text-sm font-medium text-zinc-500 bg-zinc-900 px-3 py-1 rounded-full whitespace-nowrap self-start">
                            {timeAgo(event.timestamp)}
                          </div>
                        </div>
                      </CardContent>
                    </Card>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </AppShell>
  );
}
