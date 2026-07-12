'use client';

import { Brand } from '@/components/brand';
import { SearchTrigger, FullSearchTrigger } from 'fumadocs-ui/layouts/shared/slots/search-trigger';
import { SidebarTrigger } from 'fumadocs-ui/layouts/docs/slots/sidebar';
import { GitFork, PanelLeft } from 'lucide-react';
import Link from 'next/link';
import type { ComponentProps } from 'react';

export function OperatorHeader(props: ComponentProps<'header'>) {
  return (
    <header {...props} className="operator-header">
      <Link href="/docs" className="operator-brand" aria-label="plex2jellyfin documentation home">
        <Brand />
      </Link>
      <div className="operator-header-actions">
        <div className="operator-search-full">
          <FullSearchTrigger hideIfDisabled />
        </div>
        <SearchTrigger hideIfDisabled aria-label="Open search" className="operator-search-icon" />
        <a
          className="operator-icon-link"
          href="https://github.com/nomadcxx/plex2jellyfin"
          aria-label="plex2jellyfin on GitHub"
          title="GitHub repository"
        >
          <GitFork aria-hidden="true" />
        </a>
        <SidebarTrigger className="operator-sidebar-trigger" aria-label="Open navigation">
          <PanelLeft aria-hidden="true" />
        </SidebarTrigger>
      </div>
    </header>
  );
}
