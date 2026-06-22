export type ConsolidationFilterItem = {
  id?: number;
  title?: string;
  year?: number;
  mediaType?: string;
  locations?: string[];
  targetLocation?: string;
};

export function filterConsolidationItems<T extends ConsolidationFilterItem>(items: T[], query: string): T[] {
  const q = query.trim().toLowerCase();
  if (!q) return items;

  return items.filter((item) => {
    const haystack = [
      item.title,
      item.year,
      item.mediaType,
      item.targetLocation,
      ...(item.locations ?? []),
    ]
      .filter(Boolean)
      .join(' ')
      .toLowerCase();
    return haystack.includes(q);
  });
}
