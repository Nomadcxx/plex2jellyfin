'use client';

import * as React from 'react';
import { cn } from '@/lib/utils';

interface SwitchProps extends Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, 'onChange'> {
  checked?: boolean;
  onCheckedChange?: (checked: boolean) => void;
}

const Switch = React.forwardRef<HTMLButtonElement, SwitchProps>(
  ({ className, checked = false, onCheckedChange, disabled, ...props }, ref) => {
    return (
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        data-state={checked ? 'checked' : 'unchecked'}
        disabled={disabled}
        ref={ref}
        onClick={() => !disabled && onCheckedChange?.(!checked)}
        className={cn(
          'peer inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full border-2 border-transparent',
          'transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-terminal-cyan focus-visible:ring-offset-2 focus-visible:ring-offset-zinc-950',
          'disabled:cursor-not-allowed disabled:opacity-50',
          checked ? 'bg-terminal-cyan' : 'bg-zinc-700',
          className,
        )}
        {...props}
      >
        <span
          className={cn(
            'pointer-events-none block h-4 w-4 rounded-full bg-white shadow-lg ring-0 transition-transform',
            checked ? 'translate-x-4' : 'translate-x-0',
          )}
        />
      </button>
    );
  },
);

Switch.displayName = 'Switch';

export { Switch };
