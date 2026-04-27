import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Badge } from '@/components/ui/badge';
import { components } from '@/types/api';

type QueueItem = components['schemas']['QueueItem'];

export type QueueFilterType = 'all' | 'active' | 'stuck' | 'completed';

interface QueueFilterProps {
  items: QueueItem[];
  activeFilter: QueueFilterType;
  onFilterChange: (filter: QueueFilterType) => void;
}

export function QueueFilter({ items, activeFilter, onFilterChange }: QueueFilterProps) {
  const activeCount = items.filter(item => {
    const status = item.status?.toLowerCase() || '';
    return status.includes('downloading') || status.includes('queued');
  }).length;

  const stuckCount = items.filter(item => item.isStuck).length;

  const completedCount = items.filter(item => {
    const status = item.status?.toLowerCase() || '';
    return status.includes('completed') || status.includes('finished');
  }).length;

  return (
    <Tabs value={activeFilter} onValueChange={(value) => onFilterChange(value as QueueFilterType)}>
      <TabsList className="grid w-full grid-cols-4 bg-accent/50">
        <TabsTrigger value="all" className="data-[state=active]:bg-background">
          All
          <Badge variant="secondary" className="ml-2 px-2">
            {items.length}
          </Badge>
        </TabsTrigger>
        
        <TabsTrigger value="active" className="data-[state=active]:bg-background">
          Active
          {activeCount > 0 && (
            <Badge variant="secondary" className="ml-2 px-2 bg-blue-500/20 text-blue-400">
              {activeCount}
            </Badge>
          )}
        </TabsTrigger>
        
        <TabsTrigger value="stuck" className="data-[state=active]:bg-background">
          Stuck
          {stuckCount > 0 && (
            <Badge variant="secondary" className="ml-2 px-2 bg-yellow-500/20 text-yellow-400">
              {stuckCount}
            </Badge>
          )}
        </TabsTrigger>
        
        <TabsTrigger value="completed" className="data-[state=active]:bg-background">
          Completed
          {completedCount > 0 && (
            <Badge variant="secondary" className="ml-2 px-2 bg-green-500/20 text-green-400">
              {completedCount}
            </Badge>
          )}
        </TabsTrigger>
      </TabsList>
    </Tabs>
  );
}

export function filterQueueItems(items: QueueItem[], filter: QueueFilterType): QueueItem[] {
  switch (filter) {
    case 'active':
      return items.filter(item => {
        const status = item.status?.toLowerCase() || '';
        return status.includes('downloading') || status.includes('queued');
      });
    case 'stuck':
      return items.filter(item => item.isStuck);
    case 'completed':
      return items.filter(item => {
        const status = item.status?.toLowerCase() || '';
        return status.includes('completed') || status.includes('finished');
      });
    case 'all':
    default:
      return items;
  }
}
