'use client';

import { useRecentlyAdded } from '@/hooks/useRecentlyAdded';
import { recentlyAddedImageURL } from '@/lib/api/client';

export function RecentlyAdded() {
  const { data } = useRecentlyAdded();
  if (!data?.enabled) return null;

  const items = data.items ?? [];
  if (items.length === 0 && !data.error) return null;

  return (
    <div className="mt-8">
      <h2 className="mb-4 text-xl font-semibold">Recently Added</h2>
      {data.error ? (
        <p className="text-sm text-zinc-500">{data.error}</p>
      ) : (
        <div className="-mx-1 flex gap-3 overflow-x-auto px-1 pb-2">
          {items.map((item) => {
            const label = item.type === 'Episode' && item.series_name ? item.series_name : item.name;
            return (
              <figure key={item.id} className="w-[120px] shrink-0">
                <div className="aspect-[2/3] overflow-hidden rounded-md border border-zinc-800 bg-zinc-900">
                  {/* eslint-disable-next-line @next/next/no-img-element -- same-origin API proxy; cookies required */}
                  <img
                    src={recentlyAddedImageURL(item.image_item_id)}
                    alt={label}
                    className="h-full w-full object-cover"
                    loading="lazy"
                  />
                </div>
                <figcaption className="mt-2 truncate text-xs text-zinc-300" title={label}>
                  {label}
                </figcaption>
              </figure>
            );
          })}
        </div>
      )}
    </div>
  );
}
