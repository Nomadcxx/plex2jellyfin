import { describe, expect, it } from 'vitest';
import { filterDuplicateGroups, sortDuplicateGroups } from './duplicateFilters';

const groups = [
  {
    id: 'low',
    title: 'Alpha',
    reclaimableBytes: 100,
    files: [{ path: '/tv/Alpha/Alpha.S01E01.720p.mkv' }],
  },
  {
    id: 'high',
    title: 'Beta',
    reclaimableBytes: 500,
    files: [{ path: '/movies/Beta.2160p.mkv' }, { path: '/movies/Beta.1080p.mkv' }],
  },
];

describe('duplicateFilters', () => {
  it('filters duplicate groups by title or path', () => {
    expect(filterDuplicateGroups(groups, 'beta').map((group) => group.id)).toEqual(['high']);
    expect(filterDuplicateGroups(groups, 'S01E01').map((group) => group.id)).toEqual(['low']);
  });

  it('sorts duplicate groups by reclaimable bytes descending by default', () => {
    expect(sortDuplicateGroups(groups, 'reclaimable').map((group) => group.id)).toEqual(['high', 'low']);
  });
});
