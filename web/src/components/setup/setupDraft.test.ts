import { describe, expect, it } from 'vitest';
import type { SetupDraft } from '@/hooks/useSetup';
import {
  emptyChecks,
  hydrateDraft,
  pathCheckKey,
  serviceFingerprint,
  splitPathInput,
  stepErrors,
} from './setupDraft';

function draft(): SetupDraft {
  return hydrateDraft({
    watch: { tv: [], movies: [] },
    libraries: { tv: [], movies: [] },
    sonarr: { enabled: false, url: '', api_key: '' },
    radarr: { enabled: false, url: '', api_key: '' },
    jellyfin: { enabled: false, url: '', api_key: '', path_mappings: [] },
    ai: { enabled: false, endpoint: 'http://localhost:11434', primary_model: '', fallback_model: '' },
    runtime: {
      scan_frequency: '5m',
      delete_source: true,
      verify_checksums: false,
      permissions: { user: '', group: '', file_mode: '', dir_mode: '' },
    },
  });
}

describe('setup draft validation', () => {
  it('normalizes missing collections and clears container permissions', () => {
    const got = hydrateDraft({ runtime: { permissions: { user: 'jellyfin', group: 'media', file_mode: '0644', dir_mode: '0755' } } }, 'container');
    expect(got.watch.tv).toEqual([]);
    expect(got.libraries.movies).toEqual([]);
    expect(got.runtime.permissions).toEqual({ user: '', group: '', file_mode: '', dir_mode: '' });
  });

  it('allows either TV or Movies when its pair is complete and checked', () => {
    for (const media of ['tv', 'movies'] as const) {
      const value = draft();
      value.watch[media] = [`/watch/${media}`];
      value.libraries[media] = [`/library/${media}`];
      const checks = emptyChecks();
      checks.paths[pathCheckKey('watch', media, value.watch[media][0])] = true;
      checks.paths[pathCheckKey('libraries', media, value.libraries[media][0])] = true;
      expect(stepErrors('media', value, checks, 'native')).toEqual([]);
    }
  });

  it('rejects blank, half-configured, and unchecked media paths', () => {
    const value = draft();
    expect(stepErrors('media', value, emptyChecks(), 'native')).not.toEqual([]);
    value.watch.tv = ['/watch/tv'];
    expect(stepErrors('media', value, emptyChecks(), 'native').join(' ')).toMatch(/TV library/i);
    value.libraries.tv = ['/library/tv'];
    expect(stepErrors('media', value, emptyChecks(), 'native').join(' ')).toMatch(/verify/i);
  });

  it('splits CLI-style comma and newline path lists', () => {
    expect(splitPathInput('/mnt/STORAGE1/MOVIES, /mnt/STORAGE2/MOVIES, /mnt/STORAGE3/MOVIES')).toEqual([
      '/mnt/STORAGE1/MOVIES',
      '/mnt/STORAGE2/MOVIES',
      '/mnt/STORAGE3/MOVIES',
    ]);
    expect(splitPathInput('/a\n/b\n/a, /c')).toEqual(['/a', '/b', '/c']);
    expect(splitPathInput('  ,  \n')).toEqual([]);
  });

  it('requires enabled services to be tested after their latest edit', () => {
    const value = draft();
    value.sonarr = { enabled: true, url: 'http://sonarr:8989', api_key: 'secret' };
    const checks = emptyChecks();
    expect(stepErrors('services', value, checks, 'native').join(' ')).toMatch(/Test Sonarr/i);
    checks.services.sonarr = serviceFingerprint(value.sonarr);
    expect(stepErrors('services', value, checks, 'native')).toEqual([]);
    value.sonarr.url = 'http://changed:8989';
    expect(stepErrors('services', value, checks, 'native').join(' ')).toMatch(/Test Sonarr/i);
  });

  it('requires a tested AI endpoint and distinct selected models', () => {
    const value = draft();
    value.ai.enabled = true;
    const checks = emptyChecks();
    expect(stepErrors('ai', value, checks, 'native').join(' ')).toMatch(/primary model/i);
    value.ai.primary_model = 'qwen:7b';
    value.ai.fallback_model = 'qwen:7b';
    checks.ai = value.ai.endpoint;
    expect(stepErrors('ai', value, checks, 'native').join(' ')).toMatch(/Fallback/i);
    value.ai.fallback_model = 'mistral:7b';
    expect(stepErrors('ai', value, checks, 'native')).toEqual([]);
  });
});
