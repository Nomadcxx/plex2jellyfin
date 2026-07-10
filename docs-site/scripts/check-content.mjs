import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import { extname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../content/docs/', import.meta.url));
const expectedPages = [
  'index.mdx',
  'getting-started/docker.md',
  'getting-started/installation.md',
  'getting-started/migration-guide.md',
  'reference/cli.md',
  'reference/configuration.md',
  'reference/daemon-services.md',
  'troubleshooting.md',
];

function contentFiles(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    return entry.isDirectory() ? contentFiles(path) : [path];
  });
}

for (const page of expectedPages) {
  assert.doesNotThrow(() => readFileSync(join(root, page)), `missing documentation page: ${page}`);
}

for (const path of contentFiles(root).filter((file) => ['.md', '.mdx'].includes(extname(file)))) {
  const content = readFileSync(path, 'utf8');
  assert.doesNotMatch(content, /\]\([^)]*\.md(?:#[^)]*)?\)/, `stale .md link in ${path}`);
  assert.doesNotMatch(content, /docs-src|mkdocs/i, `stale documentation system reference in ${path}`);
  assert.doesNotMatch(content, /^!!!/m, `legacy admonition syntax in ${path}`);
}

console.log(`content check passed (${expectedPages.length} pages)`);
