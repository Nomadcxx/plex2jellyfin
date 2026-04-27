import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ConfirmDestructive } from './ConfirmDestructive';

describe('ConfirmDestructive', () => {
  it('disables confirm until match', () => {
    const onConfirm = vi.fn();
    render(<ConfirmDestructive open phrase="media.db" onConfirm={onConfirm} onCancel={vi.fn()}>
      Delete the media database?
    </ConfirmDestructive>);
    const button = screen.getByRole('button', { name: /confirm/i });
    expect(button).toBeDisabled();
    fireEvent.change(screen.getByPlaceholderText(/type/i), { target: { value: 'media.db' } });
    expect(button).not.toBeDisabled();
    fireEvent.click(button);
    expect(onConfirm).toHaveBeenCalled();
  });
});
