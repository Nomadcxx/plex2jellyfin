'use client';

import { useRecentlyAdded } from '@/hooks/useRecentlyAdded';
import { RecentlyAddedItem, recentlyAddedImageURL } from '@/lib/api/client';

function formatAddedAt(iso?: string): string | null {
  if (!iso) return null;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return null;
  return d.toLocaleString(undefined, {
    day: 'numeric',
    month: 'numeric',
    year: 'numeric',
    hour: 'numeric',
    minute: 'numeric',
  });
}

function Card({ item }: { item: RecentlyAddedItem }) {
  const isEpisode = item.type === 'Episode';
  const title = isEpisode && item.series_name ? item.series_name : item.name;
  const added = formatAddedAt(item.date_created);
  const seasonEpisode =
    isEpisode && item.season_number != null && item.episode_number != null
      ? `S${item.season_number} - E${item.episode_number}`
      : null;

  return (
    <figure className="flex w-[150px] shrink-0 flex-col overflow-hidden rounded-lg bg-zinc-900">
      <div className="flex h-[220px] w-[150px] items-center justify-center overflow-hidden bg-zinc-950">
        {/* eslint-disable-next-line @next/next/no-img-element -- same-origin API proxy; cookies required */}
        <img
          src={recentlyAddedImageURL(item.image_item_id)}
          alt={title}
          className="h-full w-full object-cover transition-opacity hover:opacity-50"
          loading="lazy"
        />
      </div>
      <figcaption className="space-y-1 px-2.5 py-2.5">
        {added && <p className="text-[11px] text-cyan-400/90">{added}</p>}
        <p className="truncate text-sm text-zinc-100" title={title}>
          {title}
        </p>
        {isEpisode && (
          <p className="truncate text-xs text-zinc-500" title={item.name}>
            {item.name}
          </p>
        )}
        {seasonEpisode && <p className="pb-1 text-xs text-zinc-500">{seasonEpisode}</p>}
      </figcaption>
    </figure>
  );
}

export function RecentlyAdded() {
  const { data } = useRecentlyAdded();
  if (!data?.enabled) return null;

  const items = data.items ?? [];
  if (items.length === 0 && !data.error) return null;

  return (
    <div className="mt-8">
      <h2 className="mb-3 text-xl font-semibold">Recently Added</h2>
      {data.error ? (
        <p className="text-sm text-zinc-500">{data.error}</p>
      ) : (
        <div className="recently-added-scroll min-h-[300px] overflow-x-auto rounded-lg bg-zinc-900/60 p-5">
          <div className="flex gap-5">
            {items.map((item) => (
              <Card key={item.id} item={item} />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
