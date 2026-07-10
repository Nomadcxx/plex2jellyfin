'use client';

import { useState } from 'react';
import { ShieldCheck } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { useChangePassword } from '@/hooks/useAuth';
import { displayErrorMessage } from '@/lib/errorMessage';

export default function SecuritySettingsPage() {
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [error, setError] = useState('');
  const [success, setSuccess] = useState(false);
  const changePassword = useChangePassword();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setSuccess(false);

    if (newPassword.length < 8) {
      setError('New password must be at least 8 characters');
      return;
    }
    if (newPassword !== confirm) {
      setError('New passwords do not match');
      return;
    }

    try {
      await changePassword.mutateAsync({ currentPassword, newPassword });
      setSuccess(true);
      setCurrentPassword('');
      setNewPassword('');
      setConfirm('');
    } catch (err) {
      setError(displayErrorMessage(err, 'Could not change the password'));
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ShieldCheck className="h-5 w-5" />
          Security
        </CardTitle>
        <CardDescription>
          Change the admin password protecting this web UI. Changing it signs
          out every other session.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-4 max-w-md">
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
          {success && (
            <Alert>
              <AlertDescription>Password changed.</AlertDescription>
            </Alert>
          )}
          <div className="space-y-2">
            <label className="text-sm text-zinc-400" htmlFor="current-password">
              Current password
            </label>
            <Input
              id="current-password"
              type="password"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
              disabled={changePassword.isPending}
              autoComplete="current-password"
            />
          </div>
          <div className="space-y-2">
            <label className="text-sm text-zinc-400" htmlFor="new-password">
              New password
            </label>
            <Input
              id="new-password"
              type="password"
              placeholder="Min. 8 characters"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              disabled={changePassword.isPending}
              autoComplete="new-password"
            />
          </div>
          <div className="space-y-2">
            <label className="text-sm text-zinc-400" htmlFor="confirm-password">
              Confirm new password
            </label>
            <Input
              id="confirm-password"
              type="password"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
              disabled={changePassword.isPending}
              autoComplete="new-password"
            />
          </div>
          <Button
            type="submit"
            disabled={changePassword.isPending || !currentPassword || !newPassword || !confirm}
          >
            {changePassword.isPending ? 'Saving…' : 'Change password'}
          </Button>
          <p className="text-xs text-zinc-500">
            Locked out? Delete the <code>password_hash</code> line from{' '}
            <code>config.toml</code> on the server and reload — the first-run
            setup screen will ask for a new password.
          </p>
        </form>
      </CardContent>
    </Card>
  );
}
