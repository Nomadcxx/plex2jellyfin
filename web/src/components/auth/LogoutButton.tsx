'use client';

import { LogOut } from 'lucide-react';
import { useLogout } from '@/hooks/useAuth';

type LogoutButtonProps = {
  className?: string;
  label?: string;
  iconOnly?: boolean;
};

export function LogoutButton({
  className = '',
  label = 'Log out',
  iconOnly = false,
}: LogoutButtonProps) {
  const logout = useLogout();

  const handleClick = async () => {
    try {
      await logout.mutateAsync();
      window.location.replace('/');
    } catch {
      // Leave the user on the page; they can retry.
    }
  };

  return (
    <button
      type="button"
      onClick={handleClick}
      disabled={logout.isPending}
      className={className}
      aria-label={label}
    >
      <LogOut className="h-4 w-4 shrink-0" />
      {!iconOnly && <span>{logout.isPending ? 'Signing out…' : label}</span>}
    </button>
  );
}
