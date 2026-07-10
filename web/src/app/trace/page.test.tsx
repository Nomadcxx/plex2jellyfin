import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import TracePage from './page';

vi.mock('next/navigation', () => ({
  usePathname: () => '/trace',
  useRouter: () => ({ push: vi.fn(), refresh: vi.fn() }),
}));

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <TracePage />
    </QueryClientProvider>
  );
}

describe('TracePage', () => {
  it('renders a file journey from the trace endpoint', async () => {
    renderPage();
    expect(await screen.findByText('Show.S01E01.1080p.mkv')).toBeInTheDocument();
    expect(screen.getByText(/matched item jf-1/)).toBeInTheDocument();
    expect(screen.getByText(/Season 01/)).toBeInTheDocument();
    expect(screen.getByText('Success')).toBeInTheDocument();
  });
});
