'use client';

import { AppShell } from '@/components/layout/AppShell';
import {
  useScattered,
  useConsolidateItem,
  usePreviewConsolidation,
  type ConsolidationPreview,
} from '@/hooks/useConsolidation';
import Image from 'next/image';
import { FolderSync, ArrowRight, ArrowDown, Eye } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Card } from '@/components/ui/card';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Input } from '@/components/ui/input';
import { toast } from 'sonner';
import { formatBytes } from '@/lib/utils';
import { displayErrorMessage } from '@/lib/errorMessage';
import { useRef, useState } from 'react';
import { filterConsolidationItems } from './consolidationFilters';

export default function ConsolidationPage() {
  const { data, isLoading, error } = useScattered();
  const consolidateMutation = useConsolidateItem();
  const previewMutation = usePreviewConsolidation();
  const [activeConsolidations, setActiveConsolidations] = useState<Set<number>>(new Set());
  const activeConsolidationsRef = useRef<Set<number>>(new Set());
  const [previewItemId, setPreviewItemId] = useState<number | null>(null);
  const [previewData, setPreviewData] = useState<ConsolidationPreview | null>(null);
  const [query, setQuery] = useState('');
  // G10: confirm before firing a direct consolidation (without preview).
  const [confirmItemId, setConfirmItemId] = useState<number | null>(null);

  const handlePreview = (itemId: number) => {
    setPreviewItemId(itemId);
    setPreviewData(null);
    previewMutation.mutate(itemId, {
      onSuccess: (data) => {
        setPreviewData(data);
      },
      onError: (err) => {
        toast.error(displayErrorMessage(err, 'Failed to generate preview'));
        setPreviewItemId(null);
      },
    });
  };

  const handleClosePreview = () => {
    setPreviewItemId(null);
    setPreviewData(null);
  };

  const handleConsolidateFromPreview = () => {
    if (previewItemId === null) return;
    handleConsolidate(previewItemId);
    handleClosePreview();
  };

  const handleConsolidate = (itemId: number) => {
    if (activeConsolidationsRef.current.has(itemId)) return;
    activeConsolidationsRef.current.add(itemId);
    setActiveConsolidations(prev => new Set(prev).add(itemId));
    consolidateMutation.mutate(itemId, {
      onSuccess: () => {
        toast.success('Consolidation completed');
        activeConsolidationsRef.current.delete(itemId);
        setActiveConsolidations(prev => {
          const next = new Set(prev);
          next.delete(itemId);
          return next;
        });
      },
      onError: (err) => {
        toast.error(displayErrorMessage(err, 'Failed to consolidate'));
        activeConsolidationsRef.current.delete(itemId);
        setActiveConsolidations(prev => {
          const next = new Set(prev);
          next.delete(itemId);
          return next;
        });
      },
    });
  };

  if (isLoading) {
    return (
      <AppShell>
        <div className="space-y-8 max-w-5xl mx-auto">
          <div className="flex items-center gap-3">
            <div className="h-10 w-10 bg-zinc-900/60 rounded-lg animate-pulse" />
            <div className="h-8 w-48 bg-zinc-900/60 rounded animate-pulse" />
          </div>
          <div className="space-y-6">
            <div className="h-64 vision-card animate-pulse" />
            <div className="h-64 vision-card animate-pulse" />
          </div>
        </div>
      </AppShell>
    );
  }

  if (error) {
    return (
      <AppShell>
        <div className="p-6 bg-rose-500/10 border border-rose-500/20 rounded-md max-w-5xl mx-auto">
          <h3 className="text-lg font-medium text-rose-400">Failed to load scattered series</h3>
          <p className="text-zinc-400 mt-1">Please try again later or check the server connection.</p>
        </div>
      </AppShell>
    );
  }

  const items = data?.items || [];
  // P7: server returns totalMoves=0; derive from items so the badge reflects
  // reality.
  const totalMoves =
    data?.totalMoves && data.totalMoves > 0
      ? data.totalMoves
      : items.reduce((sum, it) => sum + (it.filesToMove || 0), 0);
  const totalBytes =
    data?.totalBytes && data.totalBytes > 0
      ? data.totalBytes
      : items.reduce((sum, it) => sum + (it.bytesToMove || 0), 0);
  const confirmItem = confirmItemId !== null ? items.find((it) => it.id === confirmItemId) : null;
  const visibleItems = filterConsolidationItems(items, query);

  return (
    <AppShell>
      <div className="space-y-6 pb-12">
        {/* Header Section */}
        <div className="flex flex-col md:flex-row md:items-end justify-between gap-6">
          <div>
            <h1 className="text-3xl font-bold flex items-center gap-3">
              <div className="p-2.5 bg-terminal-cyan/10 text-terminal-cyan rounded-md">
                <FolderSync className="h-6 w-6" />
              </div>
              Library Consolidation
            </h1>
            <p className="text-zinc-400 mt-2 max-w-xl">
              Fix scattered series that have episodes spread across multiple library folders.
              Consolidating brings all seasons and episodes into a single directory.
            </p>
          </div>
          
          {items.length > 0 && (
            <div className="flex gap-3">
              <div className="bg-zinc-950/60 p-3 rounded border border-zinc-800 text-right">
                <p className="text-sm font-medium text-zinc-500">Series to Fix</p>
                <p className="text-2xl font-bold text-sky-400">{items.length}</p>
              </div>
              <div className="bg-zinc-950/60 p-3 rounded border border-zinc-800 text-right">
                <p className="text-sm font-medium text-zinc-500">Files to Move</p>
                <p className="text-2xl font-bold text-emerald-400">{totalMoves}</p>
              </div>
            </div>
          )}
        </div>

        {items.length === 0 ? (
          <div className="vision-card-secondary flex flex-col items-center justify-center p-12 text-center min-h-[400px]">
            <div className="h-48 w-48 mb-6 relative">
              <Image
                src="/illustrations/empty-consolidation.svg"
                alt="Perfectly organized"
                fill
                className="object-contain"
              />
            </div>
            <h3 className="text-2xl font-bold text-zinc-200">Perfectly Organized</h3>
            <p className="text-zinc-400 mt-3 max-w-md mx-auto">
              All your series are neatly contained in their respective folders. No scattered files found!
            </p>
          </div>
        ) : (
          <div className="space-y-3">
            <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
              <Input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Search title, source, or target path"
                className="sm:max-w-md"
              />
              <p className="text-xs text-zinc-500">
                Showing {visibleItems.length} of {items.length} items
              </p>
            </div>
            {visibleItems.length === 0 ? (
              <div className="rounded border border-zinc-800 bg-zinc-950/50 p-6 text-sm text-zinc-400">
                No scattered media items match the current filter.
              </div>
            ) : visibleItems.map((item) => {
              const isConsolidating = item.id !== undefined && activeConsolidations.has(item.id);
              
              return (
                <Card key={item.id} className="overflow-hidden border-zinc-800/60 bg-zinc-950/50 hover:border-zinc-700 transition-colors">
                  <div className="flex h-full">
                    <div className="flex-1 p-4">
                      <div className="flex flex-col md:flex-row md:items-start justify-between gap-6">
                        <div className="flex-1 space-y-4">
                          <div>
                            <div className="flex items-center gap-3">
                              <h3 className="text-xl font-bold text-zinc-100">{item.title}</h3>
                              {item.year && (
                                <span className="text-zinc-500 font-medium">({item.year})</span>
                              )}
                            </div>
                            <div className="flex items-center gap-3 mt-2">
                              <Badge variant="outline" className="text-sky-400 border-sky-500/20 bg-sky-500/10">
                                {item.filesToMove} episodes to move
                              </Badge>
                              <span className="text-sm text-zinc-500">{formatBytes(item.bytesToMove || 0)}</span>
                            </div>
                          </div>

                          {/* Path Visualization */}
                          <div className="space-y-3 bg-zinc-950/50 p-3 rounded border border-zinc-800/50">
                            <div>
                              <p className="text-xs font-semibold text-zinc-500 uppercase tracking-wider mb-2">Current Locations</p>
                              <div className="space-y-1.5">
                                {item.locations?.map((loc, idx) => (
                                  <div key={idx} className="flex items-center gap-2 text-sm text-zinc-400 font-mono bg-zinc-900/60 p-1.5 px-3 rounded">
                                    <span className="text-rose-400 shrink-0">{"-"}</span>
                                    <span className="truncate">{loc}</span>
                                  </div>
                                ))}
                              </div>
                            </div>
                            
                            <div className="flex justify-center -my-1 relative z-10">
                              <div className="bg-zinc-800 rounded-full p-1 border border-zinc-700">
                                <ArrowDown className="h-4 w-4 text-zinc-400" />
                              </div>
                            </div>

                            <div>
                              <p className="text-xs font-semibold text-zinc-500 uppercase tracking-wider mb-2">Target Location</p>
                              <div className="flex items-center gap-2 text-sm text-emerald-400 font-mono bg-emerald-500/5 p-1.5 px-3 rounded border border-emerald-500/10">
                                <span className="text-emerald-500 shrink-0">{"+"}</span>
                                <span className="truncate">{item.targetLocation}</span>
                              </div>
                            </div>
                          </div>
                        </div>

                        <div className="flex-shrink-0 flex items-center md:items-end flex-col justify-end pt-2 gap-2">
                          <Button
                            variant="outline"
                            size="lg"
                            className="w-full md:w-auto border-zinc-700 hover:border-sky-500/50"
                            onClick={() => item.id !== undefined && handlePreview(item.id)}
                            disabled={isConsolidating || (previewMutation.isPending && previewItemId === item.id)}
                          >
                            <Eye className="mr-2 h-4 w-4" />
                            {previewMutation.isPending && previewItemId === item.id ? 'Loading...' : 'Preview'}
                          </Button>
                          <Button 
                            size="lg"
                            className="w-full md:w-auto bg-sky-600 hover:bg-sky-500 text-white shadow-lg shadow-sky-500/20 relative overflow-hidden group"
                            onClick={() => item.id !== undefined && setConfirmItemId(item.id)}
                            disabled={isConsolidating}
                          >
                            <span className="relative z-10 flex items-center">
                              {isConsolidating ? (
                                <>Consolidating...<ArrowRight className="ml-2 h-4 w-4 animate-pulse" /></>
                              ) : (
                                <>Consolidate Series<ArrowRight className="ml-2 h-4 w-4 group-hover:translate-x-1 transition-transform" /></>
                              )}
                            </span>
                            {/* Animated progress background */}
                            {isConsolidating && (
                              <div className="absolute inset-0 bg-sky-500/20 w-full animate-pulse" />
                            )}
                          </Button>
                        </div>
                      </div>
                    </div>
                  </div>
                </Card>
              );
            })}
          </div>
        )}
      </div>

      <Dialog open={previewItemId !== null} onOpenChange={(open) => !open && handleClosePreview()}>
        <DialogContent className="max-w-3xl">
          <DialogHeader>
            <DialogTitle>Consolidation Preview</DialogTitle>
            <DialogDescription>
              These moves will be performed when you confirm. No files have been modified yet.
            </DialogDescription>
          </DialogHeader>
          {previewMutation.isPending || !previewData ? (
            <div className="py-12 text-center text-zinc-400">Generating plan…</div>
          ) : (
            <div className="space-y-4">
              <div className="flex flex-wrap gap-3 text-sm">
                <Badge variant="outline" className="text-emerald-400 border-emerald-500/20 bg-emerald-500/10">
                  {previewData.filesMoved} files
                </Badge>
                <Badge variant="outline" className="text-sky-400 border-sky-500/20 bg-sky-500/10">
                  {formatBytes(previewData.bytesMoved || 0)}
                </Badge>
                <span className="text-zinc-500 font-mono truncate">→ {previewData.targetPath}</span>
              </div>
              <ScrollArea className="h-[360px] rounded-md border border-zinc-800 bg-zinc-950/60 p-3">
                {previewData.moves.length === 0 ? (
                  <p className="text-sm text-zinc-500">No file moves required.</p>
                ) : (
                  <ul className="space-y-2">
                    {previewData.moves.map((mv, i) => (
                      <li key={i} className="text-xs font-mono space-y-0.5 border-b border-zinc-800/60 pb-2 last:border-0">
                        <div className="flex gap-2 text-rose-400"><span className="shrink-0">-</span><span className="truncate" title={mv.from}>{mv.from}</span></div>
                        <div className="flex gap-2 text-emerald-400"><span className="shrink-0">+</span><span className="truncate" title={mv.to}>{mv.to}</span></div>
                        <div className="text-zinc-500 pl-4">{formatBytes(mv.bytes || 0)}</div>
                      </li>
                    ))}
                  </ul>
                )}
              </ScrollArea>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={handleClosePreview}>Cancel</Button>
            <Button
              className="bg-sky-600 hover:bg-sky-500"
              onClick={handleConsolidateFromPreview}
              disabled={!previewData || previewMutation.isPending}
            >
              Run Consolidation
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={confirmItemId !== null}
        onOpenChange={(open) => {
          if (!open) setConfirmItemId(null);
        }}
      >
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Consolidate {confirmItem?.title ?? 'series'}?</DialogTitle>
            <DialogDescription>
              {confirmItem
                ? `${confirmItem.filesToMove ?? 0} file${(confirmItem.filesToMove ?? 0) === 1 ? '' : 's'} (${formatBytes(confirmItem.bytesToMove || 0)}) will be moved into ${confirmItem.targetLocation || 'the target location'}.`
                : 'This action will move files between library folders.'}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmItemId(null)}>
              Cancel
            </Button>
            <Button
              className="bg-sky-600 hover:bg-sky-500"
              onClick={() => {
                if (confirmItemId !== null) handleConsolidate(confirmItemId);
                setConfirmItemId(null);
              }}
            >
              Run Consolidation
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </AppShell>
  );
}
