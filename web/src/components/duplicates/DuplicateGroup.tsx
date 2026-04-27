'use client';

import { useState } from 'react';
import { formatBytes } from '@/lib/utils';
import { Copy, Trash2, Film, Tv, CheckCircle2, AlertCircle } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { AlertDialog } from '@/components/ui/alert-dialog';
import { useDeleteDuplicate } from '@/hooks/useDashboard';
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card';
import { toast } from 'sonner';

type MediaFile = {
  id?: number;
  path?: string;
  size?: number;
  resolution?: string;
  sourceType?: string;
  qualityScore?: number;
};

type DuplicateGroupData = {
  id?: string;
  title?: string;
  year?: number;
  mediaType?: string;
  files?: MediaFile[];
  reclaimableBytes?: number;
};

interface DuplicateGroupProps {
  group: DuplicateGroupData;
}

export function DuplicateGroup({ group }: DuplicateGroupProps) {
  const deleteMutation = useDeleteDuplicate();
  const [pendingFileId, setPendingFileId] = useState<number | null>(null);
  const files = group.files || [];

  // Sort by quality score (highest first)
  const sortedFiles = [...files].sort((a, b) =>
    (b.qualityScore || 0) - (a.qualityScore || 0)
  );

  const bestFile = sortedFiles[0];
  const duplicates = sortedFiles.slice(1);

  const handleDeleteConfirm = () => {
    if (!pendingFileId) return;
    const fileId = pendingFileId;
    deleteMutation.mutate(
      { groupId: group.id || '', fileId },
      {
        onSuccess: () => toast.success('Duplicate deleted'),
        onError: (err) =>
          toast.error(`Delete failed: ${(err as Error).message ?? 'Unknown error'}`),
      }
    );
  };

  const getQualityBadge = (source: string) => {
    if (!source) return null;
    const s = source.toLowerCase();
    if (s.includes('remux')) return <Badge variant="success">REMUX</Badge>;
    if (s.includes('bluray') || s.includes('bdrip')) return <Badge variant="default">BluRay</Badge>;
    if (s.includes('web-dl') || s.includes('webdl')) return <Badge variant="info">WEB-DL</Badge>;
    if (s.includes('hdtv')) return <Badge variant="warning">HDTV</Badge>;
    return <Badge variant="outline">{source}</Badge>;
  };

  return (
    <Card className="overflow-hidden group hover:border-zinc-700 transition-colors">
      <CardHeader className="bg-zinc-900/50 pb-4 border-b border-zinc-800">
        <div className="flex items-center gap-3">
          <div className="p-2.5 rounded-xl bg-violet-500/10 text-violet-400">
            {group.mediaType === 'movie' ? (
              <Film className="h-5 w-5" />
            ) : (
              <Tv className="h-5 w-5" />
            )}
          </div>
          <div>
            <CardTitle className="text-lg flex items-center gap-2">
              {group.title}
              {group.year && <span className="text-zinc-500 font-normal">({group.year})</span>}
            </CardTitle>
            <p className="text-sm text-zinc-400 flex items-center gap-2 mt-1">
              <span>{files.length} files</span>
              <span className="text-zinc-600">•</span>
              <span className="text-emerald-400/90 font-medium">
                {formatBytes(group.reclaimableBytes || 0)} reclaimable
              </span>
            </p>
          </div>
        </div>
      </CardHeader>

      <CardContent className="p-0">
        <div className="divide-y divide-zinc-800/50">
          {/* Best file */}
          {bestFile && (
            <div className="p-4 bg-emerald-500/5 hover:bg-emerald-500/10 transition-colors relative">
              <div className="absolute left-0 top-0 bottom-0 w-1 bg-emerald-500/50" />
              <div className="flex items-center justify-between gap-4">
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <CheckCircle2 className="h-4 w-4 text-emerald-500" />
                    <p className="text-sm font-medium text-emerald-100 truncate">
                      {bestFile.path?.split('/').pop()}
                    </p>
                  </div>
                  <div className="flex items-center gap-3 mt-2">
                    <Badge variant="outline" className="text-zinc-400 bg-zinc-950/50 border-zinc-800">
                      {formatBytes(bestFile.size || 0)}
                    </Badge>
                    {bestFile.resolution && (
                      <Badge variant="outline" className="text-zinc-300 bg-zinc-900/50">
                        {bestFile.resolution}
                      </Badge>
                    )}
                    {getQualityBadge(bestFile.sourceType || '')}
                  </div>
                </div>
                <div className="flex-shrink-0">
                  <Badge variant="success" className="bg-emerald-500/10 text-emerald-400 hover:bg-emerald-500/20">
                    Keep This
                  </Badge>
                </div>
              </div>
            </div>
          )}

          {/* Duplicates */}
          {duplicates.map((file) => (
            <div key={file.id} className="p-4 bg-zinc-950/30 hover:bg-zinc-900/50 transition-colors">
              <div className="flex items-center justify-between gap-4">
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <AlertCircle className="h-4 w-4 text-rose-500/50" />
                    <p className="text-sm text-zinc-400 truncate strike-through">
                      {file.path?.split('/').pop()}
                    </p>
                  </div>
                  <div className="flex items-center gap-3 mt-2 opacity-70">
                    <Badge variant="outline" className="text-zinc-500 border-zinc-800/50">
                      {formatBytes(file.size || 0)}
                    </Badge>
                    {file.resolution && (
                      <Badge variant="outline" className="text-zinc-500 border-zinc-800/50">
                        {file.resolution}
                      </Badge>
                    )}
                    {getQualityBadge(file.sourceType || '')}
                  </div>
                </div>

                <Button
                  variant="outline"
                  size="sm"
                  className="text-rose-400 border-rose-500/20 hover:bg-rose-500/10 hover:text-rose-300 hover:border-rose-500/30 flex-shrink-0"
                  onClick={() => file.id && setPendingFileId(file.id)}
                  disabled={deleteMutation.isPending}
                >
                  <Trash2 className="h-4 w-4 mr-2" />
                  Delete
                </Button>
              </div>
            </div>
          ))}
        </div>
      </CardContent>

      <AlertDialog
        open={pendingFileId !== null}
        onOpenChange={(open) => !open && setPendingFileId(null)}
        title="Delete duplicate file?"
        description="This will permanently delete the file from disk. This action cannot be undone."
        confirmLabel="Delete"
        onConfirm={handleDeleteConfirm}
        destructive
      />
    </Card>
  );
}
