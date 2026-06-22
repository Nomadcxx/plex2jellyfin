import { describe, expect, it } from 'vitest';
import { filterConsolidationItems } from './consolidationFilters';

const items = [
  {
    id: 1,
    title: 'Ghosts',
    locations: ['/mnt/STORAGE1/TV/Ghosts'],
    targetLocation: '/mnt/STORAGE5/TV/Ghosts',
  },
  {
    id: 2,
    title: 'Utopia',
    locations: ['/mnt/STORAGE1/TV/Utopia AU'],
    targetLocation: '/mnt/STORAGE6/TV/Utopia',
  },
];

describe('filterConsolidationItems', () => {
  it('filters by title, source path, or target path', () => {
    expect(filterConsolidationItems(items, 'ghosts').map((item) => item.id)).toEqual([1]);
    expect(filterConsolidationItems(items, 'storage6').map((item) => item.id)).toEqual([2]);
    expect(filterConsolidationItems(items, 'storage1').map((item) => item.id)).toEqual([1, 2]);
  });
});
