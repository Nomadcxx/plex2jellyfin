import { render, screen } from '@testing-library/react';
import type { ReactNode } from 'react';
import { describe, expect, it, vi } from 'vitest';
import DashboardPage from './page';

vi.mock('next/image', () => ({
  default: (props: any) => <img {...props} />,
}));

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: any) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}));

vi.mock('@/components/layout/AppShell', () => ({
  AppShell: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

vi.mock('@/hooks/useDashboard', () => ({
  useDashboard: () => ({
    data: {
      libraryStats: {
        totalFiles: 10,
        totalSize: 1024,
        duplicateGroups: 2,
        scatteredSeries: 1,
        movieCount: 3,
        seriesCount: 4,
        episodeCount: 5,
      },
      mediaManagers: [],
    },
    isLoading: false,
    isError: false,
    error: null,
  }),
  useDuplicates: () => ({ data: { groups: [] } }),
  useJellyfinIdentification: () => ({ data: null }),
}));

describe('DashboardPage', () => {
  it('links duplicate summary to the duplicate review workflow', () => {
    render(<DashboardPage />);

    expect(screen.getByText('Duplicates').closest('a')).toHaveAttribute('href', '/duplicates');
  });
});
