import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, expect, it, vi, beforeEach } from 'vitest';
import { MobileNav, Sidebar } from './Sidebar';

const mutateAsync = vi.fn();

vi.mock('next/navigation', () => ({
  usePathname: () => '/duplicates',
}));

vi.mock('next/image', () => ({
  default: (props: { alt?: string }) => <img alt={props.alt ?? ''} />,
}));

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}));

vi.mock('@/hooks/useAuth', () => ({
  useLogout: () => ({
    mutateAsync,
    isPending: false,
  }),
}));

function renderWithQuery(ui: React.ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>);
}

describe('Sidebar', () => {
  beforeEach(() => {
    mutateAsync.mockReset();
    mutateAsync.mockResolvedValue(undefined);
  });

  it('links to the jellyfin identification page', () => {
    renderWithQuery(<Sidebar />);
    expect(screen.getByRole('link', { name: /jellyfin/i })).toHaveAttribute('href', '/jellyfin');
  });

  it('logs out from the sidebar footer', async () => {
    const replace = vi.fn();
    vi.stubGlobal('location', { ...window.location, replace });

    renderWithQuery(<Sidebar />);
    fireEvent.click(screen.getByRole('button', { name: /log out/i }));

    await waitFor(() => expect(mutateAsync).toHaveBeenCalled());
    expect(replace).toHaveBeenCalledWith('/');
    vi.unstubAllGlobals();
  });
});

describe('MobileNav', () => {
  beforeEach(() => {
    mutateAsync.mockReset();
    mutateAsync.mockResolvedValue(undefined);
  });

  it('provides duplicate workflow navigation on small screens', () => {
    renderWithQuery(<MobileNav />);

    const duplicates = screen.getByRole('link', { name: /duplicates/i });
    expect(duplicates).toHaveAttribute('href', '/duplicates');
    expect(duplicates).toHaveAttribute('aria-current', 'page');
  });

  it('includes jellyfin and a logout control', () => {
    renderWithQuery(<MobileNav />);
    expect(screen.getByRole('link', { name: /jellyfin/i })).toHaveAttribute('href', '/jellyfin');
    expect(screen.getByRole('button', { name: /log out/i })).toBeInTheDocument();
  });
});
