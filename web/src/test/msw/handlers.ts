import { http, HttpResponse } from 'msw';

export const handlers = [
  // enabled:true = a password is configured; enabled:false now renders the
  // first-run SetupForm, so tests default to a configured, signed-in state.
  http.get('/api/v1/auth/status', () =>
    HttpResponse.json({ enabled: true, authenticated: true })
  ),
  http.get('/api/v1/dashboard', () =>
    HttpResponse.json({
      libraryStats: { totalFiles: 100, totalSize: 1073741824, duplicateGroups: 3, scatteredSeries: 2 },
      mediaManagers: [{ id: 'sonarr', name: 'Sonarr', type: 'sonarr', online: true }],
    })
  ),
  http.get('/api/v1/files/trace', () =>
    HttpResponse.json({
      items: [
        {
          id: 1,
          source_path: '/downloads/tv/Show.S01E01.1080p.mkv',
          source_filename: 'Show.S01E01.1080p.mkv',
          event_at: new Date().toISOString(),
          parse_method: 'regex',
          parsed_title: 'Show',
          parsed_year: 2019,
          parsed_season: 1,
          parsed_episode: 1,
          stripped_tokens: '1080p',
          target_path: '/media/TV Shows/Show (2019)/Season 01/Show (2019) S01E01.mkv',
          target_at: new Date().toISOString(),
          outcome: 'SUCCESS',
          jellyfin: { item_id: 'jf-1', tvdb_id: '789' },
        },
      ],
    })
  ),
  http.get('/api/v1/activity', () =>
    HttpResponse.json({
      events: [
        { id: '1', type: 'FILE_ORGANIZED', message: 'Test activity', timestamp: new Date().toISOString() },
      ],
      total: 1,
    })
  ),
  http.get('/api/v1/scattered', () =>
    HttpResponse.json({
      items: [],
      totalItems: 0,
      totalMoves: 0,
      totalBytes: 0,
    })
  ),
];
