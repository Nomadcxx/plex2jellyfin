import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthGuard } from './AuthGuard';

const replace = vi.fn();
let pathname = '/';
let authState: { auth?: { enabled: boolean; authenticated: boolean }; isLoading: boolean };
let setupState: { data?: { required: boolean; complete: boolean }; isLoading: boolean; isError: boolean; refetch: () => void };

vi.mock('next/navigation', () => ({
  usePathname: () => pathname,
  useRouter: () => ({ replace }),
}));

vi.mock('@/hooks/useAuth', () => ({
  useAuth: () => authState,
}));

vi.mock('@/hooks/useSetup', () => ({
  useSetupStatus: () => setupState,
}));

vi.mock('./SetupForm', () => ({ SetupForm: () => <div>password setup</div> }));
vi.mock('./LoginForm', () => ({ LoginForm: () => <div>login form</div> }));

describe('AuthGuard setup routing', () => {
  beforeEach(() => {
    pathname = '/';
    replace.mockReset();
    authState = { auth: { enabled: true, authenticated: true }, isLoading: false };
    setupState = { data: { required: false, complete: true }, isLoading: false, isError: false, refetch: vi.fn() };
  });

  it('keeps password creation ahead of application setup', () => {
    authState = { auth: { enabled: false, authenticated: true }, isLoading: false };
    render(<AuthGuard><div>dashboard</div></AuthGuard>);
    expect(screen.getByText('password setup')).toBeInTheDocument();
    expect(screen.queryByText('dashboard')).not.toBeInTheDocument();
  });

  it('shows login before checking setup for an unauthenticated install', () => {
    authState = { auth: { enabled: true, authenticated: false }, isLoading: false };
    render(<AuthGuard><div>dashboard</div></AuthGuard>);
    expect(screen.getByText('login form')).toBeInTheDocument();
  });

  it('redirects incomplete installs away from dashboard content', () => {
    setupState.data = { required: true, complete: false };
    render(<AuthGuard><div>dashboard</div></AuthGuard>);
    expect(replace).toHaveBeenCalledWith('/setup');
    expect(screen.queryByText('dashboard')).not.toBeInTheDocument();
  });

  it('renders the wizard on the setup route while setup is required', () => {
    pathname = '/setup';
    setupState.data = { required: true, complete: false };
    render(<AuthGuard><div>setup wizard</div></AuthGuard>);
    expect(screen.getByText('setup wizard')).toBeInTheDocument();
    expect(replace).not.toHaveBeenCalled();
  });

  it('treats the static-export trailing slash as the setup route', () => {
    pathname = '/setup/';
    setupState.data = { required: true, complete: false };
    render(<AuthGuard><div>setup wizard</div></AuthGuard>);
    expect(screen.getByText('setup wizard')).toBeInTheDocument();
    expect(replace).not.toHaveBeenCalled();
  });

  it('renders configured application pages', () => {
    render(<AuthGuard><div>dashboard</div></AuthGuard>);
    expect(screen.getByText('dashboard')).toBeInTheDocument();
  });

  it('redirects configured installs away from setup', () => {
    pathname = '/setup';
    render(<AuthGuard><div>setup wizard</div></AuthGuard>);
    expect(replace).toHaveBeenCalledWith('/');
    expect(screen.queryByText('setup wizard')).not.toBeInTheDocument();
  });

  it('shows a retry state when setup status cannot be loaded', () => {
    setupState.isError = true;
    render(<AuthGuard><div>dashboard</div></AuthGuard>);
    expect(screen.getByRole('button', { name: /retry/i })).toBeInTheDocument();
    expect(screen.queryByText('dashboard')).not.toBeInTheDocument();
  });
});
