#!/usr/bin/env node
/**
 * Showcase recorder — web setup (mocked) + real dashboard.
 *
 * Capture uses Chrome CDP screencast → ffmpeg (not Playwright's soft
 * recordVideo). Default master is 1920×1080 @ deviceScaleFactor 2, CRF 16
 * H.264 MP4 — sharp source you can compress later.
 *
 * Usage:
 *   cd scripts/showcase && npm i && npx playwright install chromium
 *   SHOWCASE_PASSWORD='…' npm run record-web
 *
 * Env:
 *   SHOWCASE_BASE_URL   default http://127.0.0.1:5522
 *   SHOWCASE_PASSWORD   required for login
 *   SHOWCASE_OUT        default ./out
 *   SHOWCASE_WIDTH      default 1920
 *   SHOWCASE_HEIGHT     default 1080
 *   SHOWCASE_SCALE      deviceScaleFactor, default 2 (crisper UI)
 *   SHOWCASE_CRF        x264 quality, default 16 (lower = larger/sharper)
 *   SHOWCASE_FPS        encode fps floor, default 30
 */
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { mkdir, writeFile, rm } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { tmpdir } from 'node:os';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const BASE = process.env.SHOWCASE_BASE_URL || 'http://127.0.0.1:5522';
const PASSWORD = process.env.SHOWCASE_PASSWORD || '';
const OUT = path.resolve(process.env.SHOWCASE_OUT || path.join(__dirname, 'out'));
const WIDTH = Number(process.env.SHOWCASE_WIDTH || 1920);
const HEIGHT = Number(process.env.SHOWCASE_HEIGHT || 1080);
const SCALE = Number(process.env.SHOWCASE_SCALE || 2);
const CRF = Number(process.env.SHOWCASE_CRF || 16);
const FPS = Number(process.env.SHOWCASE_FPS || 30);

const emptyDraft = {
  watch: { tv: [], movies: [] },
  libraries: { tv: [], movies: [] },
  sonarr: { enabled: false, url: 'http://localhost:8989', api_key: '' },
  radarr: { enabled: false, url: 'http://localhost:7878', api_key: '' },
  jellyfin: {
    enabled: false,
    url: 'http://localhost:8096',
    api_key: '',
    path_mappings: [],
    plugin_install: true,
    plugin_restart: false,
    plugin_daemon_url: '',
  },
  ai: { enabled: false, endpoint: 'http://localhost:11434', primary_model: '', fallback_model: '' },
  runtime: {
    scan_frequency: '5m',
    delete_source: true,
    verify_checksums: false,
    permissions: { user: '', group: '', file_mode: '0644', dir_mode: '0755' },
  },
};

const mockStatus = {
  required: true,
  complete: false,
  daemon_state: 'stopped',
  runtime: { kind: 'native', uid: 1000, gid: 1000 },
  draft: emptyDraft,
  default_callback_url: 'http://127.0.0.1:5522/api/v1/jellyfin/plugin',
};

async function sleep(ms) {
  await new Promise((r) => setTimeout(r, ms));
}

/** High-quality capture via CDP JPEG screencast → ffmpeg H.264. */
class ScreencastRecorder {
  constructor(page) {
    this.page = page;
    this.frames = [];
    this.session = null;
    this.onFrame = null;
  }

  async start() {
    this.frames = [];
    this.session = await this.page.context().newCDPSession(this.page);
    this.onFrame = async (frame) => {
      this.frames.push({
        buf: Buffer.from(frame.data, 'base64'),
        ts: frame.metadata?.timestamp ?? Date.now() / 1000,
      });
      try {
        await this.session.send('Page.screencastFrameAck', { sessionId: frame.sessionId });
      } catch {
        /* session may already be closed */
      }
    };
    this.session.on('Page.screencastFrame', this.onFrame);
    await this.session.send('Page.startScreencast', {
      format: 'jpeg',
      quality: 92,
      maxWidth: Math.round(WIDTH * SCALE),
      maxHeight: Math.round(HEIGHT * SCALE),
      everyNthFrame: 1,
    });
  }

  async stop(outFile) {
    if (this.session) {
      try {
        await this.session.send('Page.stopScreencast');
      } catch {
        /* ignore */
      }
      this.session.off('Page.screencastFrame', this.onFrame);
      try {
        await this.session.detach();
      } catch {
        /* ignore */
      }
      this.session = null;
    }
    if (this.frames.length < 2) {
      throw new Error(`screencast captured only ${this.frames.length} frame(s)`);
    }
    await encodeFrames(this.frames, outFile);
    this.frames = [];
  }
}

async function encodeFrames(frames, outFile) {
  const dir = path.join(tmpdir(), `p2j-showcase-${process.pid}-${Date.now()}`);
  await mkdir(dir, { recursive: true });
  try {
    const listPath = path.join(dir, 'frames.txt');
    const lines = [];
    for (let i = 0; i < frames.length; i++) {
      const name = `f${String(i).padStart(6, '0')}.jpg`;
      await writeFile(path.join(dir, name), frames[i].buf);
      const nextTs = i + 1 < frames.length ? frames[i + 1].ts : frames[i].ts + 1 / FPS;
      let dur = Math.max(1 / FPS, nextTs - frames[i].ts);
      // Cap runaway gaps (tab throttle) so the cut doesn't freeze for seconds.
      if (dur > 0.5) dur = 1 / FPS;
      lines.push(`file '${name}'`);
      lines.push(`duration ${dur.toFixed(4)}`);
    }
    // concat demuxer needs the last file repeated without duration
    lines.push(`file 'f${String(frames.length - 1).padStart(6, '0')}.jpg'`);
    await writeFile(listPath, lines.join('\n'));

    await runFFmpeg([
      '-y',
      '-f', 'concat',
      '-safe', '0',
      '-i', listPath,
      '-vf', `scale=${WIDTH}:${HEIGHT}:flags=lanczos,fps=${FPS}`,
      '-c:v', 'libx264',
      '-preset', 'slow',
      '-crf', String(CRF),
      '-pix_fmt', 'yuv420p',
      '-movflags', '+faststart',
      outFile,
    ]);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
}

function runFFmpeg(args) {
  return new Promise((resolve, reject) => {
    const child = spawn('ffmpeg', args, { stdio: ['ignore', 'ignore', 'pipe'] });
    let err = '';
    child.stderr.on('data', (d) => {
      err += d.toString();
    });
    child.on('close', (code) => {
      if (code === 0) resolve();
      else reject(new Error(`ffmpeg exited ${code}\n${err.slice(-800)}`));
    });
  });
}

/** Path-verify toasts sit over the footer and steal clicks. */
async function dismissToasts(page) {
  const toasts = page.locator('[data-sonner-toast]');
  const n = await toasts.count();
  for (let i = 0; i < n; i++) {
    await toasts.nth(i).click({ force: true }).catch(() => {});
  }
  await page.evaluate(() => {
    document.querySelectorAll('[data-sonner-toast]').forEach((el) => el.remove());
  }).catch(() => {});
  await sleep(200);
}

async function clickContinue(page) {
  await dismissToasts(page);
  const btn = page.getByRole('button', { name: /Continue/i });
  await btn.click({ force: true, timeout: 10000 });
}

async function installSetupMocks(page) {
  await page.route('**/api/v1/setup/status', async (route) => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(mockStatus) });
  });
  await page.route('**/api/v1/paths/preflight', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        exists: true,
        is_dir: true,
        readable: true,
        writable: true,
        warnings: [],
      }),
    });
  });
  await page.route('**/api/v1/setup/apply', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        applied: true,
        complete: false,
        indexing: true,
        daemon_state: 'running',
      }),
    });
  });
  await page.route('**/api/v1/setup/index/stream', async (route) => {
    const libs = [
      '/mnt/STORAGE1/TVSHOWS',
      '/mnt/STORAGE2/TVSHOWS',
      '/mnt/STORAGE5/TVSHOWS',
      '/mnt/STORAGE1/MOVIES',
      '/mnt/STORAGE2/MOVIES',
      '/mnt/STORAGE4/MOVIES',
    ];
    const frames = [
      { type: 'status', phase: 'indexing', msg: 'Indexing libraries', libraries_total: libs.length },
    ];
    let scanned = 0;
    for (let i = 0; i < libs.length; i++) {
      scanned += 900 + i * 400;
      frames.push({
        type: 'progress',
        phase: 'indexing',
        library: libs[i],
        libraries_done: i,
        libraries_total: libs.length,
        files_scanned: scanned,
      });
      frames.push({
        type: 'progress',
        phase: 'indexing',
        library: libs[i],
        libraries_done: i + 1,
        libraries_total: libs.length,
        files_scanned: scanned,
      });
    }
    frames.push({
      type: 'done',
      phase: 'complete',
      msg: 'Setup complete',
      files_scanned: scanned,
      files_added: scanned,
      files_updated: 0,
      files_skipped: 12,
      episode_rows: Math.round(scanned * 0.82),
      movie_rows: Math.round(scanned * 0.18),
      duration_ms: 5400,
      daemon_state: 'running',
      libraries_done: libs.length,
      libraries_total: libs.length,
    });
    const body = frames.map((f) => `data: ${JSON.stringify(f)}\n\n`).join('');
    await route.fulfill({
      status: 200,
      headers: {
        'content-type': 'text-event-stream',
        'cache-control': 'no-cache',
        connection: 'keep-alive',
      },
      body,
    });
  });
}

async function clearSetupMocks(page) {
  await page.unroute('**/api/v1/setup/status');
  await page.unroute('**/api/v1/paths/preflight');
  await page.unroute('**/api/v1/setup/apply');
  await page.unroute('**/api/v1/setup/index/stream');
}

async function login(page) {
  await page.goto(BASE + '/');
  if (await page.getByRole('heading', { name: /Media paths|Dashboard/i }).isVisible({ timeout: 2500 }).catch(() => false)) {
    return;
  }
  if (!PASSWORD) {
    throw new Error('SHOWCASE_PASSWORD is required (dashboard login)');
  }
  const password = page.locator('input[type="password"]');
  await password.first().waitFor({ timeout: 10000 });
  await password.first().fill(PASSWORD);
  await page.locator('button[type="submit"]').or(page.getByRole('button', { name: /sign in|log in|unlock|continue|access/i })).first().click();
  await sleep(800);
}

async function fillPath(input, value) {
  await input.scrollIntoViewIfNeeded();
  await input.fill(value);
  const addBtn = input.locator('xpath=following-sibling::button[1]');
  await addBtn.click();
  for (let i = 0; i < 100; i++) {
    if (!(await addBtn.isDisabled())) break;
    await sleep(100);
  }
  await sleep(300);
  await dismissToasts(input.page());
}

async function fillMediaSection(page, media, watchPath, libraryPaths) {
  const section = page.locator(`section[aria-labelledby="${media}-paths"]`);
  await section.waitFor({ state: 'visible', timeout: 15000 });
  await section.scrollIntoViewIfNeeded();
  await fillPath(section.locator('input[aria-label="Add incoming path"]'), watchPath);
  await fillPath(section.locator('input[aria-label="Add library path"]'), libraryPaths);
}

async function runSetup(page) {
  await page.getByRole('heading', { name: /Media paths/i }).waitFor({ timeout: 20000 });
  await sleep(900);

  await fillMediaSection(
    page,
    'tv',
    '/mnt/NVME3/Sabnzbd/complete/tv',
    '/mnt/STORAGE1/TVSHOWS, /mnt/STORAGE2/TVSHOWS, /mnt/STORAGE5/TVSHOWS',
  );
  await fillMediaSection(
    page,
    'movies',
    '/mnt/NVME3/Sabnzbd/complete/movies',
    '/mnt/STORAGE1/MOVIES, /mnt/STORAGE2/MOVIES, /mnt/STORAGE4/MOVIES',
  );
  await sleep(700);
  await dismissToasts(page);

  await clickContinue(page);
  await page.getByRole('heading', { name: /Connected services/i }).waitFor();
  await sleep(600);
  await clickContinue(page);

  await page.getByRole('heading', { name: /AI matching/i }).waitFor();
  await sleep(600);
  await clickContinue(page);

  await page.getByRole('heading', { name: /Runtime behavior/i }).waitFor();
  await sleep(600);
  await clickContinue(page);

  await page.getByRole('heading', { name: /Review configuration/i }).waitFor();
  await sleep(1000);
  await dismissToasts(page);
  await page.getByRole('button', { name: /Apply and start/i }).click({ force: true });

  await page.getByRole('heading', { name: /Initial library scan/i }).waitFor();
  await page.getByRole('heading', { name: /Setup complete/i }).waitFor({ timeout: 60000 });
  await sleep(2200);
}

async function runDashboard(page) {
  await page.getByRole('heading', { name: 'Dashboard' }).waitFor({ timeout: 15000 });
  await sleep(1800);
  await page.evaluate(async () => {
    const pause = (ms) => new Promise((r) => setTimeout(r, ms));
    const max = Math.max(0, document.body.scrollHeight - window.innerHeight);
    for (let y = 0; y <= max; y += 36) {
      window.scrollTo(0, y);
      await pause(45);
    }
    await pause(1400);
    window.scrollTo(0, 0);
    await pause(900);
  });
}

async function withContext(browser, initShowcase, fn) {
  const context = await browser.newContext({
    viewport: { width: WIDTH, height: HEIGHT },
    deviceScaleFactor: SCALE,
  });
  if (initShowcase) {
    await context.addInitScript(() => {
      sessionStorage.setItem('p2j-showcase', '1');
    });
  }
  const page = await context.newPage();
  try {
    return await fn(page);
  } finally {
    await context.close();
  }
}

async function main() {
  if (!PASSWORD) {
    console.error('Set SHOWCASE_PASSWORD to the dashboard password.');
    process.exit(1);
  }
  await mkdir(OUT, { recursive: true });
  console.log(`capture ${WIDTH}x${HEIGHT} scale=${SCALE} crf=${CRF} → ${OUT}`);

  const browser = await chromium.launch({ headless: true });

  // --- Phase A: mocked setup ---
  await withContext(browser, true, async (page) => {
    const rec = new ScreencastRecorder(page);
    await installSetupMocks(page);
    await login(page);
    if (!(await page.getByRole('heading', { name: /Media paths/i }).isVisible().catch(() => false))) {
      await page.goto(BASE + '/setup');
    }
    await rec.start();
    await runSetup(page);
    const dest = path.join(OUT, '01-setup.mp4');
    await rec.stop(dest);
    console.log('wrote', dest);
  });

  // --- Phase B: real dashboard ---
  await withContext(browser, false, async (page) => {
    const rec = new ScreencastRecorder(page);
    await login(page);
    await clearSetupMocks(page).catch(() => {});
    await page.goto(BASE + '/');
    await rec.start();
    await runDashboard(page);
    const dest = path.join(OUT, '02-dashboard.mp4');
    await rec.stop(dest);
    console.log('wrote', dest);
  });

  await browser.close();
  console.log(`
Done. Master clips (high quality) in ${OUT}
Join for README (setup at 2×, then dashboard):
  ffmpeg -y -i ${OUT}/01-setup.mp4 -i ${OUT}/02-dashboard.mp4 \\
    -filter_complex '[0:v]setpts=0.5*PTS[s];[s][1:v]concat=n=2:v=1:a=0' \\
    -c:v libx264 -crf 18 -preset medium ../../assets/showcase.mp4
`);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
