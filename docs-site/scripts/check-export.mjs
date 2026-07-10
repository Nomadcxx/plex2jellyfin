import assert from 'node:assert/strict';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const output = new URL('../out/', import.meta.url);
const basePath = process.env.GITHUB_ACTIONS === 'true' ? '/plex2jellyfin' : '';
const rootHtml = readFileSync(new URL('index.html', output), 'utf8');
const docsHtml = readFileSync(new URL('docs/index.html', output), 'utf8');
const articleHtml = readFileSync(new URL('docs/reference/configuration/index.html', output), 'utf8');
const searchPath = new URL('api/search', output);
const chunkRoot = fileURLToPath(new URL('_next/static/chunks/', output));

function files(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    return entry.isDirectory() ? files(path) : [path];
  });
}

assert.match(docsHtml, /brand\/p2j-mark\.png/, 'exported header is missing the P2J mark');
assert.ok(
  docsHtml.includes(`src="${basePath}/brand/p2j-mark.png"`),
  'exported P2J mark is missing the deployment base path',
);
assert.doesNotMatch(docsHtml, /Toggle Theme|light theme/i, 'dark-only export contains a theme toggle');
assert.match(
  docsHtml,
  /https:\/\/github\.com\/nomadcxx\/plex2jellyfin/,
  'exported header is missing the repository link',
);
assert.match(
  articleHtml,
  /blob\/main\/docs-site\/content\/docs\/reference\/configuration\.md/,
  'article source link does not target the migrated content',
);
assert.ok(statSync(searchPath).size > 100, 'static search payload is empty');
assert.ok(
  files(chunkRoot).some(
    (path) => path.endsWith('.js') && readFileSync(path, 'utf8').includes(`${basePath}/api/search`),
  ),
  'search client is missing the deployment base path',
);
assert.match(docsHtml, /brand\/plex2jellyfin-wordmark\.png/, 'docs home is missing the full wordmark');
assert.match(docsHtml, /curl -fsSL/, 'docs home is missing the quick-start install command');
for (const stage of ['scan', 'duplicates', 'consolidate', 'audit', 'daemon']) {
  assert.match(docsHtml, new RegExp(`data-migration-stage="${stage}"`), `docs home is missing ${stage}`);
}
for (const route of [
  '/docs/getting-started/installation/',
  '/docs/getting-started/docker/',
  '/docs/reference/configuration/',
  '/docs/reference/cli/',
  '/docs/troubleshooting/',
]) {
  assert.ok(docsHtml.includes(`href="${basePath}${route}"`), `docs home is missing ${route}`);
}
assert.ok(
  rootHtml.includes(`url=${basePath}/docs/`),
  'site root does not redirect to the documentation home',
);

console.log('export check passed');
