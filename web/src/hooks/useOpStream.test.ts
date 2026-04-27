import { describe, expect, it, vi, afterEach } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';

class MockEventSource {
  static instances: MockEventSource[] = [];
  onmessage: ((ev: MessageEvent) => void) | null = null;
  url: string;
  closed = false;
  constructor(url: string) { this.url = url; MockEventSource.instances.push(this); }
  close() { this.closed = true; }
  emit(data: any) { this.onmessage?.({ data: JSON.stringify(data) } as MessageEvent); }
}

describe('useOpStream', () => {
  afterEach(() => { MockEventSource.instances = []; });

  it('streams frames into events array', async () => {
    (global as any).EventSource = MockEventSource;
    const { useOpStream } = await import('./useOpStream');
    const { result } = renderHook(() => useOpStream('op-x'));
    act(() => MockEventSource.instances[0].emit({ type: 'progress', phase: 'p', msg: 'hi' }));
    await waitFor(() => expect(result.current.events.length).toBe(1));
    expect(result.current.events[0].phase).toBe('p');
  });
});
