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
