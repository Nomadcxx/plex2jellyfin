import { describe, expect, it } from 'vitest';
import { APIError, fieldErrorMessages } from './errors';

describe('APIError field details', () => {
  it('keeps the per-field detail the setup endpoints return', () => {
    const err = new APIError(422, 'UNKNOWN_ERROR', 'validation failed', [
      { field: 'watch', message: 'ordinary tv watch path "/downloads" overlaps seeding movies watch path "/downloads/movies"' },
    ]);
    expect(fieldErrorMessages(err)).toEqual([
      'watch: ordinary tv watch path "/downloads" overlaps seeding movies watch path "/downloads/movies"',
    ]);
  });

  it('reports nothing extra when the response carried no fields', () => {
    expect(fieldErrorMessages(new APIError(500, 'X', 'boom'))).toEqual([]);
    expect(fieldErrorMessages(new Error('boom'))).toEqual([]);
  });
});
