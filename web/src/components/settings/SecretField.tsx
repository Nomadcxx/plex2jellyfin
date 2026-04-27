'use client';

import { useState } from 'react';
import { Eye, EyeOff } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';

type Props = {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
};

export function SecretField({ value, onChange, placeholder }: Props) {
  const [visible, setVisible] = useState(false);

  return (
    <div className="flex gap-2">
      <Input
        type={visible ? 'text' : 'password'}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
      />
      <Button type="button" variant="outline" size="icon" onClick={() => setVisible((v) => !v)}>
        {visible ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
      </Button>
    </div>
  );
}

