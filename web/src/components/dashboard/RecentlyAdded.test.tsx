import { render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, expect, it, vi, beforeEach } from 'vitest';
import { RecentlyAdded } from './RecentlyAdded';

const getRecentlyAdded = vi.fn();

vi.mock('@/lib/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/client')>('@/lib/api/client');
  return {
    ...actual,
    getRecentlyAdded: (...args: unknown[]) => getRecentlyAdded(...args),
  };
});

function renderStrip() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <RecentlyAdded />
    </QueryClientProvider>,
  );
}

describe('RecentlyAdded', () => {
  beforeEach(() => {
    getRecentlyAdded.mockReset();
  });

  it('renders nothing when jellyfin is disabled', async () => {
    getRecentlyAdded.mockResolvedValue({ enabled: false });
    renderStrip();
    await waitFor(() => expect(getRecentlyAdded).toHaveBeenCalled());
    expect(screen.queryByRole('heading', { name: /recently added/i })).not.toBeInTheDocument();
  });

  it('renders poster cards when items are present', async () => {
    getRecentlyAdded.mockResolvedValue({
      enabled: true,
      items: [
        { id: 'm1', name: 'Film One', type: 'Movie', image_item_id: 'm1', date_created: '2026-01-01T00:00:00Z' },
        { id: 'e1', name: 'Ep One', type: 'Episode', series_name: 'Show', image_item_id: 's1' },
      ],
    });

    renderStrip();
    expect(await screen.findByRole('heading', { name: /recently added/i })).toBeInTheDocument();
    expect(screen.getByText('Film One')).toBeInTheDocument();
    expect(screen.getByText('Show')).toBeInTheDocument();
    const imgs = screen.getAllByRole('img');
    expect(imgs[0]).toHaveAttribute('src', '/api/v1/jellyfin/items/m1/image/primary');
    expect(imgs[1]).toHaveAttribute('src', '/api/v1/jellyfin/items/s1/image/primary');
  });
});
