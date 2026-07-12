import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, expect, it, vi } from 'vitest';
import SettingsPage from '@/app/settings/page';

vi.mock('@/hooks/useDaemon', () => ({
  useDaemon: () => ({
    status: { state: 'running', uptime_seconds: 120, version: 'dev' },
    action: vi.fn(),
  }),
}));

vi.mock('@/hooks/useSettings', () => ({
  useSettingsSection: (section: string) => ({
    data: { enabled: section === 'jellyfin' || section === 'sonarr' },
    isLoading: false,
  }),
}));

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}));

describe('Settings overview', () => {
  it('shows a status summary without a card grid of destinations', () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <SettingsPage />
      </QueryClientProvider>,
    );

    expect(screen.getByRole('heading', { name: /overview/i })).toBeInTheDocument();
    expect(screen.getByText('running')).toBeInTheDocument();
    expect(screen.queryByText('Watch Paths')).not.toBeInTheDocument();
    expect(screen.getByText('Jellyfin').closest('a')).toHaveAttribute('href', '/settings/jellyfin');
  });
});
