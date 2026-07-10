# Fumadocs Documentation Site Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace MkDocs with a branded, dark-only Fumadocs site that exports statically to GitHub Pages.

**Architecture:** Build an independent Next.js/Fumadocs package under `docs-site/`, move the existing Markdown into its content collection, and deploy `docs-site/out/` through the current Pages workflow. Use committed transparent PNG wordmarks, static Orama search, and a repository-aware production base path.

**Tech Stack:** Next.js 16, React 19, Fumadocs Core/UI/MDX, TypeScript, Tailwind CSS 4, Orama, IBM Plex Sans/Mono, Pillow, GitHub Pages.

---

### Task 1: Scaffold the independent Fumadocs package

**Files:**
- Create: `docs-site/package.json`
- Create: `docs-site/package-lock.json`
- Create: `docs-site/tsconfig.json`
- Create: `docs-site/next-env.d.ts`
- Create: `docs-site/next.config.mjs`
- Create: `docs-site/postcss.config.mjs`
- Create: `docs-site/source.config.ts`
- Create: `docs-site/lib/source.ts`
- Create: `docs-site/lib/layout.shared.tsx`
- Create: `docs-site/mdx-components.tsx`
- Create: `docs-site/app/layout.tsx`
- Create: `docs-site/app/global.css`
- Create: `docs-site/app/docs/layout.tsx`
- Create: `docs-site/app/docs/[[...slug]]/page.tsx`
- Create: `docs-site/app/api/search/route.ts`
- Modify: `.gitignore`

**Step 1: Generate the official baseline**

Run:

```bash
npm create fumadocs-app@latest docs-site
```

Choose Next.js, Fumadocs MDX, npm, and no example content beyond the required starter page. Do not enable a hosted search provider, blog, i18n, or authentication.

**Step 2: Pin the approved dependencies**

Keep the generated versions compatible with these current package contracts:

```json
{
  "dependencies": {
    "@fontsource-variable/ibm-plex-sans": "^5.2.8",
    "@fontsource/ibm-plex-mono": "^5.2.7",
    "@orama/orama": "^3.1.18",
    "fumadocs-core": "^16.11.2",
    "fumadocs-ui": "^16.11.2",
    "fumadocs-mdx": "^15.1.0",
    "next": "^16.2.10",
    "react": "^19.2.7",
    "react-dom": "^19.2.7"
  }
}
```

Use the generator's Tailwind/PostCSS and type packages. Add only missing font and Orama packages.

**Step 3: Configure static export**

Set `docs-site/next.config.mjs` to:

```js
import { createMDX } from 'fumadocs-mdx/next';

const basePath = process.env.GITHUB_ACTIONS === 'true' ? '/plex2jellyfin' : '';

const nextConfig = {
  output: 'export',
  trailingSlash: true,
  basePath,
  images: { unoptimized: true },
};

export default createMDX()(nextConfig);
```

Use the exact import helper produced by the installed Fumadocs version if its current API differs.

**Step 4: Configure static Orama search**

Use Fumadocs' `staticGET` search export and the `oramaStaticClient`. Confirm the generated search data lives under `out/` and requires no server process.

**Step 5: Add package checks**

Define:

```json
{
  "scripts": {
    "dev": "next dev",
    "build": "next build",
    "typecheck": "tsc --noEmit",
    "check": "npm run typecheck && npm run build"
  }
}
```

Ignore `docs-site/.next/`, `docs-site/.source/`, and `docs-site/out/` explicitly rather than adding another blanket pattern.

**Step 6: Verify the baseline**

Run:

```bash
cd docs-site
npm run check
```

Expected: type checking passes and `out/docs/index.html` exists.

**Step 7: Commit**

```bash
git add .gitignore docs-site
git commit -m "build(docs): scaffold static fumadocs site"
```

### Task 2: Add reproducible ASCII brand assets

**Files:**
- Create: `docs-site/brand/plex2jellyfin.txt`
- Create: `docs-site/brand/P2J.txt`
- Create: `docs-site/public/brand/plex2jellyfin-wordmark.png`
- Create: `docs-site/public/brand/p2j-mark.png`
- Create: `scripts/render_docs_brand.py`
- Create: `scripts/test_render_docs_brand.py`

**Step 1: Write the failing standard-library test**

Test that the renderer:

- rejects empty input;
- produces RGBA output;
- preserves a transparent background;
- produces at least one visible pixel;
- creates a wider full wordmark and a compact `P2J` mark.

Use `unittest` and temporary directories. Do not add a Python test framework.

**Step 2: Run the test to verify failure**

```bash
python3 -m unittest scripts/test_render_docs_brand.py
```

Expected: FAIL because `render_docs_brand` does not exist.

**Step 3: Add the two ASCII sources**

Copy the full installer wordmark exactly from `cmd/installer/theme.go`. Copy `/home/nomadx/bit/P2J.txt` exactly, preserving UTF-8 block characters and trailing alignment.

**Step 4: Implement the Pillow renderer**

Use `PIL.Image`, `PIL.ImageDraw`, and `PIL.ImageFont`. Resolve a block-capable DejaVu Sans Mono font, measure with `textbbox`, render soft-white glyphs onto transparent RGBA, add fixed padding, and export at 2x display density. Fail with a direct message if Pillow or the font is unavailable.

Add `--check` to validate committed outputs without rewriting them.

**Step 5: Verify red-green and render outputs**

```bash
python3 -m unittest scripts/test_render_docs_brand.py
python3 scripts/render_docs_brand.py
python3 scripts/render_docs_brand.py --check
```

Expected: all commands pass.

**Step 6: Commit**

```bash
git add docs-site/brand docs-site/public/brand scripts/render_docs_brand.py scripts/test_render_docs_brand.py
git commit -m "brand(docs): add transparent ASCII wordmarks"
```

### Task 3: Migrate Markdown and navigation

**Files:**
- Move: `docs-src/index.md` -> `docs-site/content/docs/index.md`
- Move: `docs-src/getting-started/*.md` -> `docs-site/content/docs/getting-started/`
- Move: `docs-src/reference/*.md` -> `docs-site/content/docs/reference/`
- Move: `docs-src/troubleshooting.md` -> `docs-site/content/docs/troubleshooting.md`
- Create: `docs-site/content/docs/meta.json`
- Create: `docs-site/content/docs/getting-started/meta.json`
- Create: `docs-site/content/docs/reference/meta.json`
- Create: `docs-site/scripts/check-content.mjs`

**Step 1: Write a failing content check**

Use `node:fs`, `node:path`, and `node:test` or a direct assertion script. Require all eight expected pages, reject links that still contain `.md`, and reject references to `docs-src` or MkDocs.

**Step 2: Run it to verify failure**

```bash
cd docs-site
node scripts/check-content.mjs
```

Expected: FAIL while the pages remain under `docs-src/`.

**Step 3: Move content and add metadata**

Use `git mv`. Define the approved order and labels in the three `meta.json` files. Add minimal frontmatter titles/descriptions only where Fumadocs requires them.

**Step 4: Update links**

Convert relative `.md` links to route links, preserving anchors. Keep external GitHub and product links unchanged.

**Step 5: Add content check to `npm run check`**

Run the content check before type checking and building.

**Step 6: Verify**

```bash
cd docs-site
npm run check
```

Expected: content assertions, types, and static build pass.

**Step 7: Commit**

```bash
git add docs-src docs-site/content docs-site/scripts docs-site/package.json
git commit -m "docs: migrate content to fumadocs"
```

### Task 4: Implement the dark operator-console shell

**Files:**
- Modify: `docs-site/app/global.css`
- Modify: `docs-site/app/layout.tsx`
- Modify: `docs-site/app/docs/layout.tsx`
- Modify: `docs-site/lib/layout.shared.tsx`
- Create: `docs-site/components/brand.tsx`
- Create: `docs-site/components/search.tsx`

**Step 1: Add a failing rendered-shell assertion**

Extend `check-content.mjs` or add `check-export.mjs` to require:

- the `P2J` image in exported header markup;
- no light-theme toggle label;
- the repository GitHub link;
- the static search payload.

Run the build and checker. Expected: FAIL before the shell is customized.

**Step 2: Apply fonts and dark-only provider**

Import IBM Plex Sans variable weights and IBM Plex Mono regular/medium weights. Set dark color scheme at the HTML root and remove theme switching from the provider and header.

**Step 3: Implement the layout**

Use Fumadocs layout APIs first. Customize only the slots needed for:

- 56px header;
- compact `P2J` mark and product label;
- static search trigger;
- version text;
- GitHub icon link;
- desktop sidebar and page outline;
- mobile navigation drawer.

Use Lucide icons already supplied by Fumadocs UI. Add accessible labels and tooltips for icon-only controls.

**Step 4: Apply the approved CSS system**

Define the approved palette as CSS variables. Use near-black page bands, graphite surfaces, cyan focus/current states, sparse amber warnings, 4-6px corners, zero negative letter spacing, and fixed responsive layout dimensions. Check contrast and long path wrapping.

**Step 5: Verify**

```bash
cd docs-site
npm run check
```

Expected: shell assertions, types, and build pass.

**Step 6: Commit**

```bash
git add docs-site/app docs-site/components docs-site/lib
git commit -m "feat(docs): add operator-console theme"
```

### Task 5: Build the documentation home page

**Files:**
- Modify: `docs-site/content/docs/index.md` or rename to `index.mdx`
- Create: `docs-site/components/docs-home.tsx`

**Step 1: Add failing home-page export assertions**

Require the full wordmark, quick-start commands, all five migration stages, and links to Installation, Docker, Configuration, CLI, and Troubleshooting.

**Step 2: Implement the page**

Create a compact first viewport with:

- full transparent ASCII wordmark;
- one-sentence product purpose;
- quick-start command block;
- scan -> duplicates -> consolidate -> audit -> daemon workflow;
- unframed documentation link rows below it.

Do not use a marketing hero, nested cards, feature prose, gradients, or decorative background shapes.

**Step 3: Verify**

```bash
cd docs-site
npm run check
```

Expected: all home-page assertions and build pass.

**Step 4: Commit**

```bash
git add docs-site/content/docs/index.* docs-site/components/docs-home.tsx
git commit -m "feat(docs): add branded documentation home"
```

### Task 6: Replace the Pages workflow and MkDocs files

**Files:**
- Modify: `.github/workflows/docs.yml`
- Delete: `mkdocs.yml`
- Modify: `README.md`
- Modify: `.gitignore`

**Step 1: Update workflow triggers and build**

Use Node 22 with npm cache rooted at `docs-site/package-lock.json`. Run:

```yaml
- run: npm ci
  working-directory: docs-site
- run: npm run check
  working-directory: docs-site
```

Upload `docs-site/out/`. Retain `pages: write`, `id-token: write`, the `github-pages` environment, concurrency, and official configure/upload/deploy actions.

**Step 2: Remove MkDocs**

Delete `mkdocs.yml`, stop ignoring `site/` if it is no longer produced, and remove MkDocs-specific workflow setup.

**Step 3: Update public links and contributor commands**

Keep the canonical Pages URL. Replace MkDocs build commands with `cd docs-site && npm ci && npm run check`. Correct any paths changed by the content move.

**Step 4: Validate workflow syntax and build**

```bash
cd docs-site && npm run check
git diff --check
```

If `actionlint` is installed, run it against `.github/workflows/docs.yml`; otherwise inspect the workflow structure locally and rely on GitHub's parser after push.

**Step 5: Commit**

```bash
git add .github/workflows/docs.yml .gitignore README.md mkdocs.yml
git commit -m "ci(docs): deploy fumadocs to pages"
```

### Task 7: Visual and responsive verification

**Files:**
- Modify as required: `docs-site/app/global.css`
- Modify as required: `docs-site/components/*.tsx`
- Modify as required: `docs-site/content/docs/*.mdx`

**Step 1: Start the exported site**

Run the production build with `GITHUB_ACTIONS=true`, then serve `docs-site/out/` locally with a simple static server. Confirm `/plex2jellyfin/` is the entry route.

**Step 2: Capture desktop screenshots**

Use Playwright at 1440x900 for:

- home page;
- configuration article;
- search dialog open.

Verify the full wordmark, compact header mark, sidebar, content column, page outline, code blocks, and search results.

**Step 3: Capture mobile screenshots**

Use Playwright at 390x844 for:

- home page;
- article page;
- navigation drawer open.

Verify no clipped ASCII image, horizontal overflow, overlapping controls, or undersized touch targets.

**Step 4: Check rendered pixels and accessibility basics**

Confirm screenshots are nonblank, the logo alpha renders correctly, focus indicators are visible, landmarks are present, headings are ordered, and icon-only controls have accessible names.

**Step 5: Fix only observed defects and rerun `npm run check`**

Do not add visual decoration during the verification pass.

**Step 6: Commit**

```bash
git add docs-site
git commit -m "fix(docs): polish responsive documentation layout"
```

### Task 8: Repository and deployment gate

**Files:**
- No planned changes

**Step 1: Run documentation gates**

```bash
cd docs-site
npm ci
npm run check
cd ..
python3 scripts/render_docs_brand.py --check
```

**Step 2: Run existing project gates**

```bash
go build ./...
go vet ./...
go test -count=1 ./...
cd web && npm test
```

Expected: all commands pass.

**Step 3: Inspect scope**

```bash
git status --short
git diff --check
git log --oneline --max-count=10
```

Confirm unrelated review changes remain intact and generated `.next`, `.source`, or `out` directories are absent from git status.

**Step 4: Push and verify Pages**

After user approval to publish, push the commits. Watch the `Deploy Documentation` workflow, open `https://nomadcxx.github.io/plex2jellyfin/`, and verify home, article, search, assets, and mobile navigation from the deployed site.
