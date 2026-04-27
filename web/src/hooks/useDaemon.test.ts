import { describe, expect, it, vi } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { useDaemon } from './useDaemon';

describe('useDaemon', () => {
  it('polls /daemon/status', async () => {
    const fetchSpy = vi.spyOn(global, 'fetch').mockResolvedValue({
      ok: true,
      json: async () => ({ state: 'running', uptime_s: 60 }),
    } as Response);

    const { result } = renderHook(() => useDaemon(50));
    await waitFor(() => expect(result.current.status?.state).toBe('running'));
    fetchSpy.mockRestore();
  });
});
