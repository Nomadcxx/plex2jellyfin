import { APIError, AuthError, NetworkError } from './errors';

const API_BASE = '/api/v1';

async function apiRequest<T>(
  endpoint: string,
  options: RequestInit = {}
): Promise<T> {
  const url = `${API_BASE}${endpoint}`;
  
  try {
    const response = await fetch(url, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        ...options.headers,
      },
      credentials: 'include',
    });

    if (response.status === 401) {
      throw new AuthError();
    }

    if (!response.ok) {
      const data = await response.json().catch(() => ({}));
      throw new APIError(
        response.status,
        data.code || 'UNKNOWN_ERROR',
        data.message || `HTTP ${response.status}`
      );
    }

    return response.json();
  } catch (error) {
    if (error instanceof APIError || error instanceof AuthError) {
      throw error;
    }
    if (error instanceof TypeError) {
      throw new NetworkError();
    }
    throw error;
  }
}

export const api = {
  get: <T>(endpoint: string) => apiRequest<T>(endpoint, { method: 'GET' }),
  post: <T>(endpoint: string, body?: unknown) =>
    apiRequest<T>(endpoint, {
      method: 'POST',
      body: body ? JSON.stringify(body) : undefined,
    }),
  delete: <T>(endpoint: string) => apiRequest<T>(endpoint, { method: 'DELETE' }),
};

export * from './errors';
