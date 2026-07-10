import { Provider } from '@/components/provider';
import type { Metadata } from 'next';
import './global.css';

const basePath = process.env.NEXT_PUBLIC_BASE_PATH ?? '';

export const metadata: Metadata = {
  metadataBase: new URL('https://nomadcxx.github.io'),
  title: {
    default: 'plex2jellyfin documentation',
    template: '%s | plex2jellyfin',
  },
  description: 'Documentation for migrating Plex-organized media libraries to Jellyfin.',
  icons: { icon: `${basePath}/brand/p2j-mark.png` },
};

export default function Layout({ children }: LayoutProps<'/'>) {
  return (
    <html lang="en" className="dark" style={{ colorScheme: 'dark' }}>
      <body className="flex flex-col min-h-screen">
        <Provider>{children}</Provider>
      </body>
    </html>
  );
}
