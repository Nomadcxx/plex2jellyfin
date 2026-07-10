'use client';

import { usePathname } from 'next/navigation';

// The shell's signature: a terminal prompt echoing the CLI. The path names
// the current page the way `plex2jellyfin trace` names its output.
export function TermPrompt() {
  const pathname = usePathname();
  const segment = pathname === '/' ? 'dashboard' : pathname.replace(/^\//, '').replace(/\/$/, '');

  return (
    <p className="flex items-center gap-2 font-mono text-[13px] text-zinc-500" aria-hidden="true">
      <span className="text-terminal-cyan">❯</span>
      <span>
        p2j://<span className="text-terminal-amber">{segment}</span>
      </span>
      <span className="inline-block h-3.5 w-[7px] animate-pulse bg-zinc-600 motion-reduce:animate-none" />
    </p>
  );
}
