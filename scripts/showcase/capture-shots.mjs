#!/usr/bin/env node
/**
 * Capture static screenshots for README: setup wizard + dashboard.
 * SHOWCASE_PASSWORD required. Uses same mocks as record-web for setup.
 */
import { chromium } from 'playwright';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { mkdir } from 'node:fs/promises';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const BASE = process.env.SHOWCASE_BASE_URL || 'http://127.0.0.1:5522';
const PASSWORD = process.env.SHOWCASE_PASSWORD || '';
const OUT = path.resolve(process.env.SHOWCASE_SHOTS || path.join(__dirname, '../../assets'));

const emptyDraft = {
  watch: { tv: [], movies: [] },
  libraries: { tv: [], movies: [] },
  sonarr: { enabled: false, url: 'http://localhost:8989', api_key: '' },
  radarr: { enabled: false, url: 'http://localhost:7878', api_key: '' },
  jellyfin: {
    enabled: false, url: 'http://localhost:8096', api_key: '',
    path_mappings: [], plugin_install: true, plugin_restart: false, plugin_daemon_url: '',
  },
  ai: { enabled: false, endpoint: 'http://localhost:11434', primary_model: '', fallback_model: '' },
  runtime: {
    scan_frequency: '5m', delete_source: true, verify_checksums: false,
    permissions: { user: '', group: '', file_mode: '0644', dir_mode: '0755' },
  },
};

const mockStatus = {
  required: true, complete: false, daemon_state: 'stopped',
  runtime: { kind: 'native', uid: 1000, gid: 1000 },
  draft: {
    ...emptyDraft,
    watch: { tv: ['/mnt/NVME3/Sabnzbd/complete/tv'], movies: ['/mnt/NVME3/Sabnzbd/complete/movies'] },
    libraries: {
      tv: ['/mnt/STORAGE1/TVSHOWS', '/mnt/STORAGE2/TVSHOWS', '/mnt/STORAGE5/TVSHOWS'],
      movies: ['/mnt/STORAGE1/MOVIES', '/mnt/STORAGE2/MOVIES', '/mnt/STORAGE4/MOVIES'],
    },
  },
  default_callback_url: 'http://127.0.0.1:5522/api/v1/jellyfin/plugin',
};

async function sleep(ms) { await new Promise((r) => setTimeout(r, ms)); }

async function main() {
  if (!PASSWORD) throw new Error('SHOWCASE_PASSWORD required');
  await mkdir(OUT, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: { width: 1920, height: 1080 },
    deviceScaleFactor: 2,
  });
  const page = await context.newPage();

  // Dashboard (real)
  await page.goto(BASE + '/');
  if (!(await page.getByRole('heading', { name: 'Dashboard' }).isVisible({ timeout: 2000 }).catch(() => false))) {
    await page.locator('input[type="password"]').first().fill(PASSWORD);
    await page.getByRole('button', { name: /Sign In/i }).click();
    await page.getByRole('heading', { name: 'Dashboard' }).waitFor({ timeout: 15000 });
  }
  await sleep(2000);
  await page.screenshot({ path: path.join(OUT, 'screenshot-dashboard.png'), fullPage: false });
  console.log('wrote screenshot-dashboard.png');

  // Setup (mocked status so wizard shows without wiping config)
  await page.route('**/api/v1/setup/status', async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(mockStatus) });
  });
  await page.route('**/api/v1/paths/preflight', async (route) => {
    await route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({ exists: true, is_dir: true, readable: true, writable: true, warnings: [] }),
    });
  });
  await page.goto(BASE + '/setup');
  await page.getByRole('heading', { name: /Media paths/i }).waitFor({ timeout: 15000 });
  // Mark paths verified in UI state by clicking verify isn't needed if draft already has paths —
  // hydrate shows them but checks may be empty (grey circles). Still looks like setup.
  await sleep(1000);
  await page.screenshot({ path: path.join(OUT, 'screenshot-setup.png'), fullPage: false });
  console.log('wrote screenshot-setup.png');

  await browser.close();
}

main().catch((e) => { console.error(e); process.exit(1); });
