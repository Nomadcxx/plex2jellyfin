import type { ComponentProps } from 'react';

const basePath = process.env.NEXT_PUBLIC_BASE_PATH ?? '';

export function Brand({ className, ...props }: ComponentProps<'span'>) {
  return (
    <span className={`p2j-brand ${className ?? ''}`} {...props}>
      <img src={`${basePath}/brand/p2j-mark.png`} alt="" width="83" height="30" />
      <span>plex2jellyfin</span>
    </span>
  );
}
