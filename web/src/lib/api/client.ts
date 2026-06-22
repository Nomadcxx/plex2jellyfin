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
      // Global session-expiry handling: bounce to /login so users aren't
      // left staring at a raw error on a protected page. Guarded for SSR
      // and for requests that originate from the login page itself (to
      // avoid a redirect loop during a failed sign-in attempt).
      if (typeof window !== 'undefined' && !window.location.pathname.startsWith('/login')) {
        window.location.replace('/login');
      }
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

    if (response.status === 204) {
      return undefined as T;
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
  put: <T>(endpoint: string, body?: unknown) =>
    apiRequest<T>(endpoint, {
      method: 'PUT',
      body: body ? JSON.stringify(body) : undefined,
    }),
  delete: <T>(endpoint: string) => apiRequest<T>(endpoint, { method: 'DELETE' }),
};

export type SettingsSection =
  | 'paths'
  | 'libraries'
  | 'sonarr'
  | 'radarr'
  | 'jellyfin'
  | 'tmdb'
  | 'ai'
  | 'options'
  | 'logging'
  | 'permissions';

export type PathKind = 'movies' | 'tv';

export type SettingsSaveResponse = {
  saved: boolean;
  reload: {
    ok: boolean;
    reloaded?: string[];
    failed?: Array<{ name: string; error: string }>;
  };
  restored_previous_config?: boolean;
};

export type PathListResponse = {
  paths: string[];
};

export type PreflightResult = {
  path: string;
  exists: boolean;
  is_dir: boolean;
  readable: boolean;
  writable: boolean;
  owner_uid: number;
  daemon_uid_can_access: boolean;
  free_space_bytes: number;
  warnings?: string[];
};

export type ConnectionTestResult = {
  ok: boolean;
  version?: string;
  error?: string;
};

export async function getSettingsSection<T extends Record<string, unknown>>(section: SettingsSection) {
  return api.get<T>(`/settings/${section}`);
}

export async function putSettingsSection<T extends Record<string, unknown>>(section: SettingsSection, body: T) {
  return api.put<SettingsSaveResponse>(`/settings/${section}`, body);
}

export async function getSettingsPaths(collection: 'paths' | 'libraries', kind: PathKind) {
  return api.get<PathListResponse>(`/settings/${collection}/${kind}`);
}

export async function addSettingsPath(collection: 'paths' | 'libraries', kind: PathKind, path: string) {
  return api.post<PathListResponse>(`/settings/${collection}/${kind}`, { path });
}

export async function removeSettingsPath(collection: 'paths' | 'libraries', kind: PathKind, index: number) {
  await api.delete<void>(`/settings/${collection}/${kind}/${index}`);
}

export async function preflightPath(path: string, kind: 'watch' | 'library') {
  return api.post<PreflightResult>('/paths/preflight', { path, kind });
}

export async function testSettingsConnection(service: 'sonarr' | 'radarr' | 'jellyfin', payload: Record<string, unknown>) {
  return api.post<ConnectionTestResult>(`/settings/${service}/test`, payload);
}

export type OperationAcceptedResponse = {
  op_id: string;
};

export async function startDatabaseRescan(paths: string[], dryRun: boolean) {
  return api.post<OperationAcceptedResponse>('/database/rescan', { paths, dry_run: dryRun });
}

export async function startDatabaseReset(preserve: string[] = ['audit_log']) {
  return api.post<OperationAcceptedResponse>('/database/reset', {
    confirm: 'media.db',
    preserve,
  });
}

export async function reconcileJellyfinMetadata(limit: number) {
  return api.post<OperationAcceptedResponse>('/jellyfin/metadata/reconcile', { limit });
}

export async function repairJellyfinMetadata(decisionIds: number[]) {
  return api.post<OperationAcceptedResponse>('/jellyfin/metadata/repair', { decision_ids: decisionIds });
}

export async function repairJellyfinMetadataItem(id: number) {
  return api.post<OperationAcceptedResponse>(`/jellyfin/metadata/repair/${id}`, {});
}

export * from './errors';
