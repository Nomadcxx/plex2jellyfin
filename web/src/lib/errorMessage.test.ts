import { describe, expect, it } from 'vitest';
import { APIError } from './api/errors';
import { displayErrorMessage } from './errorMessage';

describe('displayErrorMessage', () => {
  it('uses API error messages instead of the generic fallback', () => {
    const error = new APIError(
      500,
      'consolidation_failed',
      'Failed to consolidate: target path is not writable'
    );

    expect(displayErrorMessage(error, 'Failed to consolidate')).toBe(
      'Failed to consolidate: target path is not writable'
    );
  });

  it('truncates very long backend errors', () => {
    const error = new Error(`Failed: ${'x'.repeat(400)}`);

    expect(displayErrorMessage(error, 'Fallback')).toHaveLength(240);
    expect(displayErrorMessage(error, 'Fallback')).toMatch(/\.\.\.$/);
  });

  it('falls back for non-error values', () => {
    expect(displayErrorMessage('boom', 'Fallback')).toBe('Fallback');
  });
});
