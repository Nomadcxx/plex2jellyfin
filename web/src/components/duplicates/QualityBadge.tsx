import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';

type QualityTier = 'REMUX' | 'BluRay' | 'WEB-DL' | 'WEBRip' | 'HDTV' | 'DVDRip' | 'Unknown';

const QUALITY_HIERARCHY: Record<QualityTier, { score: number; color: string; label: string }> = {
  'REMUX': { score: 100, color: 'bg-purple-500 text-purple-50', label: 'REMUX' },
  'BluRay': { score: 90, color: 'bg-blue-500 text-blue-50', label: 'BluRay' },
  'WEB-DL': { score: 70, color: 'bg-green-500 text-green-50', label: 'WEB-DL' },
  'WEBRip': { score: 60, color: 'bg-yellow-600 text-yellow-50', label: 'WEBRip' },
  'HDTV': { score: 40, color: 'bg-orange-500 text-orange-50', label: 'HDTV' },
  'DVDRip': { score: 20, color: 'bg-red-500 text-red-50', label: 'DVDRip' },
  'Unknown': { score: 0, color: 'bg-gray-500 text-gray-50', label: 'Unknown' },
};

const RESOLUTION_SCORES: Record<string, number> = {
  '2160p': 30,
  '4K': 30,
  'UHD': 30,
  '1080p': 20,
  '720p': 10,
  '480p': 5,
};

interface QualityBadgeProps {
  sourceType?: string;
  resolution?: string;
  qualityScore?: number;
  isBest?: boolean;
  className?: string;
}

function detectQualityTier(sourceType?: string): QualityTier {
  if (!sourceType) return 'Unknown';
  
  const upper = sourceType.toUpperCase();
  
  if (upper.includes('REMUX')) return 'REMUX';
  if (upper.includes('BLURAY') || upper.includes('BLU-RAY') || upper.includes('BDRIP')) return 'BluRay';
  if (upper.includes('WEB-DL') || upper.includes('WEBDL')) return 'WEB-DL';
  if (upper.includes('WEBRIP')) return 'WEBRip';
  if (upper.includes('HDTV')) return 'HDTV';
  if (upper.includes('DVDRIP') || upper.includes('DVD')) return 'DVDRip';
  
  return 'Unknown';
}

export function QualityBadge({ sourceType, resolution, qualityScore, isBest, className }: QualityBadgeProps) {
  const tier = detectQualityTier(sourceType);
  const qualityInfo = QUALITY_HIERARCHY[tier];
  
  const resolutionScore = resolution ? RESOLUTION_SCORES[resolution] || 0 : 0;
  const totalScore = qualityInfo.score + resolutionScore;
  
  return (
    <div className={cn('flex items-center gap-2', className)}>
      <Badge
        variant="outline"
        className={cn(
          'font-semibold border-0',
          qualityInfo.color,
          isBest && 'ring-2 ring-offset-2 ring-yellow-400 ring-offset-background'
        )}
      >
        {qualityInfo.label}
      </Badge>
      
      {resolution && (
        <Badge variant="secondary" className="font-mono text-xs">
          {resolution}
        </Badge>
      )}
      
      {isBest && (
        <Badge className="bg-yellow-500 text-yellow-950 text-xs font-bold">
          BEST
        </Badge>
      )}
      
      {qualityScore !== undefined && (
        <span className="text-xs text-muted-foreground font-mono">
          Score: {totalScore}
        </span>
      )}
    </div>
  );
}

export function getQualityScore(sourceType?: string, resolution?: string): number {
  const tier = detectQualityTier(sourceType);
  const qualityInfo = QUALITY_HIERARCHY[tier];
  const resolutionScore = resolution ? RESOLUTION_SCORES[resolution] || 0 : 0;
  
  return qualityInfo.score + resolutionScore;
}
