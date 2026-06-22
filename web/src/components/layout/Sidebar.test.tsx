import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { MobileNav } from './Sidebar';

vi.mock('next/navigation', () => ({
  usePathname: () => '/duplicates',
}));

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: any) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}));

describe('MobileNav', () => {
  it('provides duplicate workflow navigation on small screens', () => {
    render(<MobileNav />);

    const duplicates = screen.getByRole('link', { name: /duplicates/i });
    expect(duplicates).toHaveAttribute('href', '/duplicates');
    expect(duplicates).toHaveAttribute('aria-current', 'page');
  });
});
