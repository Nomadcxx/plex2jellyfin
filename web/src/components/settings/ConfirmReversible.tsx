'use client';

import { ReactNode } from 'react';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog';

type Props = {
  open: boolean;
  title: string;
  children: ReactNode;
  onConfirm: () => void;
  onCancel: () => void;
};

export function ConfirmReversible({ open, title, children, onConfirm, onCancel }: Props) {
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onCancel()}>
      <DialogContent>
        <DialogTitle>{title}</DialogTitle>
        <div className="text-sm">{children}</div>
        <div className="mt-3 flex justify-end gap-2">
          <Button variant="ghost" onClick={onCancel}>Cancel</Button>
          <Button onClick={onConfirm}>Confirm</Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
