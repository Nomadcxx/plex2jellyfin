"use client";

import { useScanStatus, useStartScan } from '@/hooks/useScan';
import { Button } from '@/components/ui/button';
import { Scan, Loader2 } from 'lucide-react';
import { toast } from 'sonner';

export function ScanButton() {
  const { data: status } = useScanStatus();
  const startScan = useStartScan();
  const isScanning = status?.status === 'scanning';

  const handleScan = () => {
    startScan.mutate(undefined, {
      onSuccess: () => toast.success('Library scan started'),
      onError: () => toast.error('Failed to start scan'),
    });
  };

  return (
    <Button onClick={handleScan} disabled={isScanning}>
      {isScanning ? <Loader2 className="h-4 w-4 mr-2 animate-spin" /> : <Scan className="h-4 w-4 mr-2" />}
      {isScanning ? `Scanning ${status?.progress || 0}%` : 'Scan Library'}
    </Button>
  );
}
