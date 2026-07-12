import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, expect, it, vi, beforeEach } from 'vitest';
import { PluginStatusPanel } from './PluginStatusPanel';

const getJellyfinPluginStatus = vi.fn();
const verifyJellyfinPlugin = vi.fn();

vi.mock('@/lib/api/client', () => ({
  getJellyfinPluginStatus: (...args: unknown[]) => getJellyfinPluginStatus(...args),
  verifyJellyfinPlugin: (...args: unknown[]) => verifyJellyfinPlugin(...args),
}));

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn(), message: vi.fn() },
}));

describe('PluginStatusPanel', () => {
  beforeEach(() => {
    getJellyfinPluginStatus.mockReset();
    verifyJellyfinPlugin.mockReset();
  });

  it('loads plugin status on Status click', async () => {
    getJellyfinPluginStatus.mockResolvedValue({
      healthy: true,
      installed: true,
      version: '1.2.3',
      message: 'ok',
    });

    render(<PluginStatusPanel />);
    fireEvent.click(screen.getByRole('button', { name: /^status$/i }));

    await waitFor(() => expect(getJellyfinPluginStatus).toHaveBeenCalled());
    expect(await screen.findByText(/1\.2\.3/)).toBeInTheDocument();
    expect(screen.getByText(/Healthy:/)).toBeInTheDocument();
  });
});
