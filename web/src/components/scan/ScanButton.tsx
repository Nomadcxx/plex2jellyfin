"use client";

import { useRouter } from 'next/navigation';
import { Button } from '@/components/ui/button';
import { Scan } from 'lucide-react';

export function ScanButton() {
  const router = useRouter();
  return (
    <Button onClick={() => router.push('/settings/indexing')}>
      <Scan className="h-4 w-4 mr-2" />
      Scan Library
    </Button>
  );
}
