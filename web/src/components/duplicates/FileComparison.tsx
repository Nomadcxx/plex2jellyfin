import { components } from '@/types/api';
import { formatBytes } from '@/lib/utils';
import { QualityBadge, getQualityScore } from './QualityBadge';
import { File, HardDrive } from 'lucide-react';
import { cn } from '@/lib/utils';

type MediaFile = components['schemas']['MediaFile'];

interface FileComparisonProps {
  files: MediaFile[];
  bestFileId?: number;
  onSelectFile?: (fileId: number) => void;
  selectedFileId?: number;
}

export function FileComparison({ files, bestFileId, onSelectFile, selectedFileId }: FileComparisonProps) {
  const sortedFiles = [...files].sort((a, b) => {
    const scoreA = getQualityScore(a.sourceType, a.resolution);
    const scoreB = getQualityScore(b.sourceType, b.resolution);
    return scoreB - scoreA;
  });

  return (
    <div className="space-y-3">
      {sortedFiles.map((file, index) => {
        const isBest = file.id === bestFileId;
        const isSelected = file.id === selectedFileId;
        const canSelect = onSelectFile !== undefined;
        
        return (
          <div
            key={file.id}
            className={cn(
              'relative p-4 rounded-lg border-2 transition-all',
              isBest && 'border-yellow-500 bg-yellow-500/5',
              !isBest && isSelected && 'border-blue-500 bg-blue-500/5',
              !isBest && !isSelected && 'border-border bg-accent/30',
              canSelect && !isBest && 'cursor-pointer hover:border-blue-400 hover:bg-blue-500/10'
            )}
            onClick={() => canSelect && file.id && onSelectFile(file.id)}
          >
            <div className="flex items-start justify-between gap-4">
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 mb-2">
                  <File className="h-4 w-4 text-muted-foreground shrink-0" />
                  <p className="text-sm font-mono text-foreground truncate" title={file.path}>
                    {file.path?.split('/').pop()}
                  </p>
                </div>
                
                <div className="flex items-center gap-4 text-xs text-muted-foreground mb-3">
                  <span className="flex items-center gap-1">
                    <HardDrive className="h-3 w-3" />
                    {formatBytes(file.size || 0)}
                  </span>
                  {file.qualityScore !== undefined && (
                    <span className="font-mono">
                      Quality: {file.qualityScore}
                    </span>
                  )}
                </div>
                
                <QualityBadge
                  sourceType={file.sourceType}
                  resolution={file.resolution}
                  qualityScore={file.qualityScore}
                  isBest={isBest}
                />
              </div>
              
              {index === 0 && !isBest && (
                <div className="text-xs text-green-400 font-semibold bg-green-500/10 px-2 py-1 rounded">
                  HIGHEST
                </div>
              )}
            </div>
            
            <div className="mt-2 text-xs text-muted-foreground truncate" title={file.path}>
              {file.path}
            </div>
          </div>
        );
      })}
    </div>
  );
}
