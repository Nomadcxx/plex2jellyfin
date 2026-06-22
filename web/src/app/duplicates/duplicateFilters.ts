export type DuplicateGroupSummary = {
  id?: string;
  title?: string;
  year?: number;
  mediaType?: string;
  reclaimableBytes?: number;
  files?: Array<{ path?: string }>;
};

export type DuplicateSort = 'reclaimable' | 'files' | 'title';

export function filterDuplicateGroups<T extends DuplicateGroupSummary>(groups: T[], query: string): T[] {
  const q = query.trim().toLowerCase();
  if (!q) return groups;

  return groups.filter((group) => {
    const haystack = [
      group.title,
      group.year,
      group.mediaType,
      ...(group.files ?? []).map((file) => file.path),
    ]
      .filter(Boolean)
      .join(' ')
      .toLowerCase();
    return haystack.includes(q);
  });
}

export function sortDuplicateGroups<T extends DuplicateGroupSummary>(groups: T[], sort: DuplicateSort): T[] {
  return [...groups].sort((a, b) => {
    if (sort === 'title') return (a.title ?? '').localeCompare(b.title ?? '');
    if (sort === 'files') return (b.files?.length ?? 0) - (a.files?.length ?? 0);
    return (b.reclaimableBytes ?? 0) - (a.reclaimableBytes ?? 0);
  });
}
