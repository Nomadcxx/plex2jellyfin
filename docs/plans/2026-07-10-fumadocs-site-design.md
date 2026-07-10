# Fumadocs Documentation Site Design

## Goal

Replace the MkDocs site with a distinct, dark-only Fumadocs site deployed as a static GitHub Pages application. Preserve the existing documentation content while giving Plex2Jellyfin its own visual identity.

## Constraints

- GitHub Pages remains the canonical host.
- The documentation frontend stays independent from the embedded `web/` dashboard.
- Existing Markdown remains the content source; use MDX only where richer components add value.
- The site must export as static files with no server runtime or hosted search dependency.
- The design is dark-only. Do not add a light palette or theme toggle.
- Keep the dependency and file count small.

## Framework

Use Fumadocs with Next.js and TypeScript in a separate `docs-site/` package. Fumadocs provides the documentation layout, content processing, static Orama search, and accessible navigation. Local layout components may be customized when CSS and supported layout slots cannot express the approved design.

The embedded dashboard under `web/` keeps its current dependencies and build process. The documentation site has its own `package.json` and lockfile so updates cannot disturb the shipped web binary.

## Content Structure

Move the eight pages under `docs-src/` to `docs-site/content/docs/`:

```text
docs-site/content/docs/
  index.md
  getting-started/
    installation.md
    docker.md
    migration-guide.md
  reference/
    cli.md
    configuration.md
    daemon-services.md
  troubleshooting.md
```

Replace the MkDocs navigation block with Fumadocs metadata files. Preserve useful URLs where possible and update internal links during the move. Delete `mkdocs.yml` and remove MkDocs from the Pages workflow after the Fumadocs build passes.

## Visual Direction

The theme is an operator console: technical and compact without imitating a terminal.

### Palette

- Page background: `#0B0D0E`
- Raised surface: `#141819`
- Secondary surface: `#1B2022`
- Primary text: `#F2F4F3`
- Muted text: `#9AA4A6`
- Border: `#2A3236`
- Primary accent: `#6FCFE0`
- Signal accent: `#D9A84E`, used only for warnings and selected details
- Success: muted mint
- Error or destructive state: muted coral

Do not use gradients, decorative blobs, oversized cards, nested cards, or large-radius containers. Use rules, compact panels, restrained 4-6px corners, and stable content widths.

### Typography

Use IBM Plex Sans for navigation, headings, and prose. Use IBM Plex Mono for code, paths, commands, and technical labels. Bundle fonts through the documentation package rather than loading them from a remote service at runtime.

### Layout

- Compact 56px header with the `P2J` mark, product name, search, version, and GitHub link.
- Desktop navigation sidebar, central article column, and right-side page outline.
- Mobile header with the compact mark, search button, and navigation drawer.
- Documentation home page uses the full ASCII wordmark, a one-sentence purpose, a quick-start command block, and the migration workflow. It opens into useful documentation rather than a marketing page.
- Article pages use tight headings, clear procedural steps, strong code blocks, and restrained callouts.

## Brand Assets

Maintain two block-character sources:

- The full Plex2Jellyfin wordmark already used by the installer.
- The compact `P2J` artwork from `/home/nomadx/bit/P2J.txt`.

Copy the compact source into the repository. Use Pillow and one block-capable monospace font to render both sources as high-resolution transparent PNGs. The full image appears on the home page. The compact mark appears in the header, mobile navigation, social metadata, and favicon source.

The renderer must fail if a source is empty, output dimensions are invalid, or the result has no non-transparent pixels. Commit the generated images so CI does not depend on system font rendering.

## Static Search

Use Fumadocs' static Orama client. Generate the search data during `next build`; the browser downloads and searches the static index. Do not add Algolia, a hosted API, or a server route.

## GitHub Pages

Use Next.js static export with:

- `output: "export"`
- `trailingSlash: true`
- `/plex2jellyfin` as the production base path
- unoptimized images where required by static export

Local development runs at `/`. Asset and navigation URLs must work both locally and under the repository subpath.

Update `.github/workflows/docs.yml` to install Node dependencies, run checks, build `docs-site/out/`, upload that directory as the Pages artifact, and deploy it with the existing Pages actions.

## Verification

- Run the documentation TypeScript check and production build.
- Fail on broken internal links or missing base-path assets.
- Confirm the static search index is present and non-empty.
- Check both PNGs for expected dimensions, alpha transparency, and non-empty visible pixels.
- Serve the exported site locally and capture desktop and mobile screenshots.
- Verify the header, navigation, content, page outline, code blocks, and search at both viewport sizes.
- Confirm no text overlaps or overflows and the site remains readable at narrow mobile widths.
- Run the repository's existing Go and frontend checks to prove the separate documentation package did not disturb shipped binaries.

## Release Boundary

The migration is complete when GitHub Pages deploys the static Fumadocs output and all public documentation links resolve under `https://nomadcxx.github.io/plex2jellyfin/`. Remove MkDocs configuration only after the replacement passes locally.
