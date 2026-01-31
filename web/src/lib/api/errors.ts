export class APIError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string
  ) {
    super(message);
    this.name = 'APIError';
  }
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
