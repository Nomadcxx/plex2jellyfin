'use client';

import { Loader2, PlugZap } from 'lucide-react';
import { Button } from '@/components/ui/button';

type Props = {
  pending: boolean;
  onClick: () => void;
};

export function TestConnectionButton({ pending, onClick }: Props) {
  return (
    <Button type="button" variant="outline" onClick={onClick} disabled={pending}>
      {pending ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <PlugZap className="mr-2 h-4 w-4" />}
      Test connection
    </Button>
  );
}

