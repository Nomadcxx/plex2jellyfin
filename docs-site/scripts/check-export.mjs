import assert from 'node:assert/strict';
import { readFileSync, statSync } from 'node:fs';

const output = new URL('../out/', import.meta.url);
const docsHtml = readFileSync(new URL('docs/index.html', output), 'utf8');
const searchPath = new URL('api/search', output);

assert.match(docsHtml, /brand\/p2j-mark\.png/, 'exported header is missing the P2J mark');
assert.doesNotMatch(docsHtml, /Toggle Theme|light theme/i, 'dark-only export contains a theme toggle');
assert.match(
  docsHtml,
  /https:\/\/github\.com\/nomadcxx\/plex2jellyfin/,
  'exported header is missing the repository link',
);
assert.match(
  docsHtml,
  /blob\/main\/docs-site\/content\/docs\/index\.md/,
  'article source link does not target the migrated content',
);
assert.ok(statSync(searchPath).size > 100, 'static search payload is empty');

console.log('export check passed');
