import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, expect, it, vi, beforeEach } from 'vitest';
import { CompatibilityPanel } from './CompatibilityPanel';

const checkArrCompatibility = vi.fn();

vi.mock('@/lib/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/client')>('@/lib/api/client');
  return {
    ...actual,
    checkArrCompatibility: (...args: unknown[]) => checkArrCompatibility(...args),
  };
});

vi.mock('@/hooks/useSettings', () => ({
  useSettingsSection: () => ({
    data: { enabled: true, url: 'http://sonarr.local', api_key: 'secret' },
    isLoading: false,
  }),
}));

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn(), message: vi.fn() },
}));

function renderPanel() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <CompatibilityPanel service="sonarr" />
    </QueryClientProvider>,
  );
}

describe('CompatibilityPanel', () => {
  beforeEach(() => {
    checkArrCompatibility.mockReset();
  });

  it('checks compatibility and lists issues', async () => {
    checkArrCompatibility.mockResolvedValue({
      ok: true,
      healthy: false,
      issues: [{ setting: 'enableCompletedDownloadHandling', current: 'true', expected: 'false', severity: 'critical' }],
    });

    renderPanel();
    fireEvent.click(screen.getByRole('button', { name: /^check$/i }));

    await waitFor(() => expect(checkArrCompatibility).toHaveBeenCalledWith(
      'sonarr',
      { url: 'http://sonarr.local', api_key: 'secret' },
      false,
    ));
    expect(await screen.findByText(/enableCompletedDownloadHandling/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^fix$/i })).toBeInTheDocument();
  });
});
