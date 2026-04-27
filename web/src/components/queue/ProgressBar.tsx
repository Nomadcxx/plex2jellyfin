import { Progress } from '@/components/ui/progress';
import { cn } from '@/lib/utils';

interface ProgressBarProps {
  progress: number;
  status?: string;
  className?: string;
  showPercentage?: boolean;
  animated?: boolean;
}

export function ProgressBar({ 
  progress, 
  status, 
  className, 
  showPercentage = true,
  animated = true 
}: ProgressBarProps) {
  const normalizedProgress = Math.min(Math.max(progress, 0), 100);
  
  const getProgressColor = () => {
    if (normalizedProgress >= 100) return 'bg-green-500';
    if (normalizedProgress >= 75) return 'bg-blue-500';
    if (normalizedProgress >= 50) return 'bg-yellow-500';
    return 'bg-orange-500';
  };

  const getStatusColor = () => {
    if (!status) return 'text-muted-foreground';
    
    const statusLower = status.toLowerCase();
    if (statusLower.includes('completed') || statusLower.includes('finished')) {
      return 'text-green-400';
    }
    if (statusLower.includes('downloading') || statusLower.includes('active')) {
      return 'text-blue-400';
    }
    if (statusLower.includes('failed') || statusLower.includes('error')) {
      return 'text-red-400';
    }
    if (statusLower.includes('stuck') || statusLower.includes('warning')) {
      return 'text-yellow-400';
    }
    return 'text-muted-foreground';
  };

  return (
    <div className={cn('space-y-2', className)}>
      <div className="flex items-center justify-between text-sm">
        {status && (
          <span className={cn('font-medium capitalize', getStatusColor())}>
            {status}
          </span>
        )}
        {showPercentage && (
          <span className="text-muted-foreground font-mono">
            {normalizedProgress.toFixed(1)}%
          </span>
        )}
      </div>
      
      <div className="relative">
        <Progress 
          value={normalizedProgress} 
          className={cn(
            'h-2',
            animated && 'transition-all duration-300 ease-in-out'
          )}
        />
        {animated && normalizedProgress < 100 && normalizedProgress > 0 && (
          <div 
            className="absolute top-0 left-0 h-full bg-white/20 rounded-full animate-pulse"
            style={{ width: `${normalizedProgress}%` }}
          />
        )}
      </div>
    </div>
  );
}
