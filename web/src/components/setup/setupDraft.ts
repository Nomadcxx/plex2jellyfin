import type { SetupDraft } from '@/hooks/useSetup';

export type SetupStep = 'media' | 'services' | 'ai' | 'runtime' | 'review';
export type RuntimeKind = 'native' | 'container';
export type ServiceName = 'sonarr' | 'radarr' | 'jellyfin';

export type WizardChecks = {
  paths: Record<string, boolean>;
  services: Partial<Record<ServiceName, string>>;
  ai?: string;
};

type DeepPartial<T> = {
  [P in keyof T]?: T[P] extends Array<infer U>
    ? Array<DeepPartial<U>>
    : T[P] extends object
      ? DeepPartial<T[P]>
      : T[P];
};

export function hydrateDraft(input: DeepPartial<SetupDraft> = {}, runtime: RuntimeKind = 'native'): SetupDraft {
  const source = input as any;
  const permissions = runtime === 'container'
    ? { user: '', group: '', file_mode: '', dir_mode: '' }
    : {
        user: source.runtime?.permissions?.user ?? '',
        group: source.runtime?.permissions?.group ?? '',
        file_mode: source.runtime?.permissions?.file_mode ?? '',
        dir_mode: source.runtime?.permissions?.dir_mode ?? '',
      };

  return {
    watch: {
      tv: [...(source.watch?.tv ?? [])],
      movies: [...(source.watch?.movies ?? [])],
    },
    libraries: {
      tv: [...(source.libraries?.tv ?? [])],
      movies: [...(source.libraries?.movies ?? [])],
    },
    sonarr: service(source.sonarr),
    radarr: service(source.radarr),
    jellyfin: {
      ...service(source.jellyfin),
      path_mappings: [...(source.jellyfin?.path_mappings ?? [])].map((mapping: any) => ({
        jellyfin: mapping.jellyfin ?? '',
        daemon: mapping.daemon ?? '',
      })),
      plugin_install: source.jellyfin?.plugin_install ?? true,
      plugin_restart: source.jellyfin?.plugin_restart ?? false,
      plugin_daemon_url: source.jellyfin?.plugin_daemon_url ?? '',
    },
    ai: {
      enabled: source.ai?.enabled ?? false,
      endpoint: source.ai?.endpoint ?? 'http://localhost:11434',
      primary_model: source.ai?.primary_model ?? '',
      fallback_model: source.ai?.fallback_model ?? '',
    },
    runtime: {
      scan_frequency: source.runtime?.scan_frequency ?? '5m',
      delete_source: source.runtime?.delete_source ?? true,
      verify_checksums: source.runtime?.verify_checksums ?? false,
      permissions,
    },
  };
}

export function emptyChecks(): WizardChecks {
  return { paths: {}, services: {} };
}

export function pathCheckKey(collection: 'watch' | 'libraries', media: 'tv' | 'movies', path: string): string {
  return `${collection}:${media}:${path.trim()}`;
}

export function serviceFingerprint(value: { url: string; api_key: string }): string {
  return `${value.url.trim()}\n${value.api_key.trim()}`;
}

export function stepErrors(step: SetupStep, draft: SetupDraft, checks: WizardChecks, runtime: RuntimeKind): string[] {
  if (step === 'review') {
    return (['media', 'services', 'ai', 'runtime'] as SetupStep[]).flatMap((part) => stepErrors(part, draft, checks, runtime));
  }
  if (step === 'media') return mediaErrors(draft, checks);
  if (step === 'services') return serviceErrors(draft, checks);
  if (step === 'ai') return aiErrors(draft, checks);

  const errors: string[] = [];
  if (!draft.runtime.scan_frequency.trim()) errors.push('Select a scan frequency.');
  if (runtime === 'native') {
    for (const [label, mode] of [
      ['File mode', draft.runtime.permissions.file_mode],
      ['Directory mode', draft.runtime.permissions.dir_mode],
    ] as const) {
      if (mode && !/^0?[0-7]{3}$/.test(mode)) errors.push(`${label} must be an octal mode such as 0644.`);
    }
  }
  return errors;
}

function mediaErrors(draft: SetupDraft, checks: WizardChecks): string[] {
  const errors: string[] = [];
  const pairs = [
    ['TV', 'tv'],
    ['Movies', 'movies'],
  ] as const;
  if (pairs.every(([, media]) => draft.watch[media].length === 0 && draft.libraries[media].length === 0)) {
    errors.push('Configure TV, Movies, or both.');
  }
  for (const [label, media] of pairs) {
    const incoming = draft.watch[media];
    const library = draft.libraries[media];
    if (incoming.length > 0 && library.length === 0) errors.push(`Add a ${label} library path.`);
    if (library.length > 0 && incoming.length === 0) errors.push(`Add a ${label} incoming path.`);
    for (const path of incoming) {
      if (!checks.paths[pathCheckKey('watch', media, path)]) errors.push(`Verify incoming path ${path}.`);
    }
    for (const path of library) {
      if (!checks.paths[pathCheckKey('libraries', media, path)]) errors.push(`Verify library path ${path}.`);
    }
  }
  return errors;
}

function serviceErrors(draft: SetupDraft, checks: WizardChecks): string[] {
  const errors: string[] = [];
  for (const name of ['sonarr', 'radarr', 'jellyfin'] as ServiceName[]) {
    const value = draft[name];
    if (!value.enabled) continue;
    const label = name[0].toUpperCase() + name.slice(1);
    if (!/^https?:\/\//.test(value.url)) errors.push(`${label} needs an HTTP URL.`);
    if (!value.api_key.trim()) errors.push(`${label} needs an API key.`);
    if (checks.services[name] !== serviceFingerprint(value)) errors.push(`Test ${label} with the current values.`);
  }
  for (const mapping of draft.jellyfin.path_mappings) {
    if (!mapping.jellyfin.trim() || !mapping.daemon.trim()) errors.push('Each Jellyfin path mapping needs both paths.');
  }
  if (draft.jellyfin.enabled && draft.jellyfin.plugin_install) {
    const url = (draft.jellyfin.plugin_daemon_url ?? '').trim();
    if (!/^https?:\/\//.test(url)) {
      errors.push('Companion plugin needs a callback URL reachable from Jellyfin (not localhost in containers).');
    }
  }
  return errors;
}

function aiErrors(draft: SetupDraft, checks: WizardChecks): string[] {
  if (!draft.ai.enabled) return [];
  const errors: string[] = [];
  if (!/^https?:\/\//.test(draft.ai.endpoint)) errors.push('Ollama needs an HTTP endpoint.');
  if (!draft.ai.primary_model.trim()) errors.push('Select a primary model.');
  if (draft.ai.fallback_model && draft.ai.fallback_model === draft.ai.primary_model) errors.push('Fallback model must differ from the primary model.');
  if (checks.ai !== draft.ai.endpoint.trim()) errors.push('Test Ollama with the current endpoint.');
  return errors;
}

function service(value: any): { enabled: boolean; url: string; api_key: string } {
  return {
    enabled: value?.enabled ?? false,
    url: value?.url ?? '',
    api_key: value?.api_key ?? '',
  };
}
