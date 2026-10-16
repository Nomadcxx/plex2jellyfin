export type FieldError = { field: string; message: string };

export class APIError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    public fields: FieldError[] = []
  ) {
    super(message);
    this.name = 'APIError';
  }
}

/** Setup validation and path preflight answer with a generic message plus the
 * per-field detail (watch-layout overlaps land here). Callers render these
 * alongside the message so the reason is visible, not just "validation failed". */
export function fieldErrorMessages(error: unknown): string[] {
  if (!(error instanceof APIError)) return [];
  return error.fields
    .filter((entry) => entry && typeof entry.message === 'string' && entry.message.trim() !== '')
    .map((entry) => (entry.field ? `${entry.field}: ${entry.message}` : entry.message));
}

export class NetworkError extends Error {
  constructor(message = 'Network error') {
    super(message);
    this.name = 'NetworkError';
  }
}

export class AuthError extends Error {
  constructor(message = 'Authentication required') {
    super(message);
    this.name = 'AuthError';
  }
}
