import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { DuplicateGroup } from './DuplicateGroup';

const mutate = vi.fn();

vi.mock('@/hooks/useDashboard', () => ({
  useDeleteDuplicate: () => ({ mutate }),
}));

vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

describe('DuplicateGroup', () => {
  it('requires typing the duplicate filename before deleting from disk', () => {
    render(
      <DuplicateGroup
        group={{
          id: 'group-1',
          title: 'Movie',
          mediaType: 'movie',
          files: [
            {
              id: 1,
              path: '/library/Movie.2160p.mkv',
              size: 2000,
              qualityScore: 100,
            },
            {
              id: 2,
              path: '/library/Movie.720p.mkv',
              size: 1000,
              qualityScore: 20,
            },
          ],
        }}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: /delete/i }));

    const confirm = screen.getByRole('button', { name: /confirm/i });
    expect(screen.getAllByText('/library/Movie.720p.mkv').length).toBeGreaterThanOrEqual(1);
    expect(confirm).toBeDisabled();

    fireEvent.change(screen.getByPlaceholderText(/type movie\.720p\.mkv/i), {
      target: { value: 'Movie.720p.mkv' },
    });
    expect(confirm).not.toBeDisabled();

    fireEvent.click(confirm);
    expect(mutate).toHaveBeenCalledWith(
      { groupId: 'group-1', fileId: 2 },
      expect.any(Object),
    );
  });
});
