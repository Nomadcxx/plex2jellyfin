import { basePath } from '@/lib/shared';
import { ArrowRight } from 'lucide-react';
import Link from 'next/link';

const stages = [
  ['01', 'scan', 'Index the library without changing files.'],
  ['02', 'duplicates', 'Review competing copies before cleanup.'],
  ['03', 'consolidate', 'Preview and merge scattered series.'],
  ['04', 'audit', 'Resolve uncertain names with explicit review.'],
  ['05', 'daemon', 'Keep new arrivals in Jellyfin-ready shape.'],
] as const;

const links = [
  ['Installation', 'Script, packages, and source builds', '/docs/getting-started/installation'],
  ['Docker', 'Container paths, ownership, and startup', '/docs/getting-started/docker'],
  ['Configuration', 'Complete config.toml reference', '/docs/reference/configuration'],
  ['CLI Reference', 'Migration, repair, and control commands', '/docs/reference/cli'],
  ['Troubleshooting', 'Permissions, mappings, parsing, and services', '/docs/troubleshooting'],
] as const;

const installOptions = [
  [
    'Guided installer',
    'Interactive Linux setup for paths, permissions, services, integrations, and the initial scan.',
    '/docs/getting-started/installation#quick-install',
  ],
  [
    'Docker',
    'Run the daemon and Web UI together with bind-mounted config, watch, and library paths.',
    '/docs/getting-started/docker',
  ],
  [
    'Deb / RPM',
    'Install release-built packages, then configure and enable the systemd services.',
    '/docs/getting-started/packages',
  ],
  [
    'Source build',
    'Build the installer locally when a release package is not suitable for the host.',
    '/docs/getting-started/installation#build-from-source',
  ],
] as const;

export function DocsHome() {
  return (
    <div className="docs-home">
      <h1 className="sr-only">plex2jellyfin documentation</h1>
      <section className="docs-home-intro" aria-labelledby="docs-home-purpose">
        <img
          className="docs-wordmark"
          src={`${basePath}/brand/plex2jellyfin-wordmark.png`}
          alt="plex2jellyfin"
          width="1591"
          height="229"
        />
        <p id="docs-home-purpose">
          Migrate a Plex-organized media library to Jellyfin, then keep every new arrival clean.
        </p>
        <nav className="install-options" aria-label="Install options">
          {installOptions.map(([title, description, href]) => (
            <Link key={title} href={href} data-install-option={title}>
              <strong>{title}</strong>
              <span>{description}</span>
              <ArrowRight aria-hidden="true" />
            </Link>
          ))}
        </nav>
      </section>

      <section className="migration-flow" aria-labelledby="migration-flow-title">
        <div className="section-heading">
          <p>OPERATOR FLOW</p>
          <h2 id="migration-flow-title">From inherited library to continuous maintenance</h2>
        </div>
        <ol>
          {stages.map(([number, stage, description]) => (
            <li key={stage} data-migration-stage={stage}>
              <span className="stage-number">{number}</span>
              <strong>{stage}</strong>
              <span>{description}</span>
            </li>
          ))}
        </ol>
      </section>

      <nav className="docs-home-links" aria-label="Documentation sections">
        <div className="section-heading">
          <p>DOCUMENTATION</p>
          <h2>Deploy, migrate, and operate</h2>
        </div>
        {links.map(([title, description, href]) => (
          <Link key={href} href={href}>
            <strong>{title}</strong>
            <span>{description}</span>
            <ArrowRight aria-hidden="true" />
          </Link>
        ))}
      </nav>
    </div>
  );
}
