import { basePath } from '@/lib/shared';

export default function HomePage() {
  const docsUrl = `${basePath}/docs/`;

  return (
    <main className="root-redirect">
      <meta httpEquiv="refresh" content={`0;url=${docsUrl}`} />
      <p>
        Opening <a href={docsUrl}>plex2jellyfin documentation</a>.
      </p>
    </main>
  );
}
