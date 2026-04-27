'use client';

import { ReactNode, useState } from 'react';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';

type Props = {
  open: boolean;
  phrase: string;
  children: ReactNode;
  onConfirm: () => void;
  onCancel: () => void;
};

export function ConfirmDestructive({ open, phrase, children, onConfirm, onCancel }: Props) {
  const [typed, setTyped] = useState('');
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onCancel()}>
      <DialogContent>
        <DialogTitle>Confirm destructive action</DialogTitle>
        <div className="text-sm">{children}</div>
        <p className="mt-3 text-sm">
          Type <span className="font-mono">{phrase}</span> to confirm.
        </p>
        <Input
          value={typed}
          placeholder={`type ${phrase}`}
          onChange={(e) => setTyped(e.target.value)}
        />
        <div className="mt-3 flex justify-end gap-2">
          <Button variant="ghost" onClick={onCancel}>Cancel</Button>
          <Button variant="destructive" disabled={typed !== phrase} onClick={onConfirm}>
            Confirm
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
