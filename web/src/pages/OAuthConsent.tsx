import { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { AlertCircle } from 'lucide-react';
import { api } from '../lib/api';
import { useAuth } from '../hooks/useAuth';
import { useTheme } from '../hooks/useTheme';

interface PendingAuth {
  rid: string;
  clientId: string;
  clientName: string;
  scope: string;
  redirectURI: string;
}

function redirectHost(uri: string): string {
  try {
    return new URL(uri).host;
  } catch {
    return uri;
  }
}

export default function OAuthConsent() {
  const { user } = useAuth();
  const { theme } = useTheme();
  const [params] = useSearchParams();
  const rid = params.get('rid') ?? '';
  const [pending, setPending] = useState<PendingAuth | null>(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState<'grant' | 'deny' | ''>('');

  useEffect(() => {
    if (!rid) {
      setError('This authorization request is missing. Go back to Cursor and try again.');
      setLoading(false);
      return;
    }
    let cancelled = false;
    api.getOAuthPending(rid)
      .then((result) => {
        if (!cancelled) setPending(result);
      })
      .catch((err: Error) => {
        if (!cancelled) setError(err.message || 'This authorization request expired. Go back to Cursor and try again.');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [rid]);

  const decide = async (decision: 'grant' | 'deny') => {
    setError('');
    setSubmitting(decision);
    try {
      const result = await api.submitOAuthConsent(rid, decision);
      window.location.assign(result.redirectUrl);
    } catch (err: any) {
      setError(err.message || 'Could not finish authorization.');
      setSubmitting('');
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-[var(--gantry-bg-secondary)] px-4">
      <div className="w-full max-w-md">
        <div className="mb-8 text-center">
          <div className="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-[var(--gantry-accent)]">
            <img src={theme === 'dark' ? '/logo-black.png' : '/logo-white.png'} alt="Gantry" className="h-10 w-10 object-contain" />
          </div>
          <h1 className="text-2xl font-bold text-[var(--gantry-text-primary)]">Authorize access</h1>
          <p className="mt-1 text-sm text-[var(--gantry-text-secondary)]">Gantry catalog</p>
        </div>

        <div className="rounded-xl border border-[var(--gantry-border)] bg-[var(--gantry-bg-primary)] p-6 shadow-sm">
          {error && (
            <div className="mb-4 flex items-center gap-2 rounded-lg bg-red-50 px-3 py-2 text-sm text-[var(--gantry-danger)] dark:bg-red-900/20">
              <AlertCircle className="h-4 w-4 shrink-0" />
              <span>{error}</span>
            </div>
          )}

          {loading && <p className="text-sm text-[var(--gantry-text-secondary)]">Loading authorization request...</p>}

          {!loading && pending && (
            <div className="space-y-4">
              <p className="text-sm text-[var(--gantry-text-primary)]">
                <span className="font-medium">{pending.clientName}</span> wants to use the Gantry catalog with your permissions.
              </p>
              <dl className="space-y-2 rounded-lg border border-[var(--gantry-border)] bg-[var(--gantry-bg-secondary)] px-3 py-3 text-sm">
                <div className="flex justify-between gap-4">
                  <dt className="text-[var(--gantry-text-secondary)]">Signed in as</dt>
                  <dd className="font-medium text-[var(--gantry-text-primary)]">{user?.displayName || user?.username}</dd>
                </div>
                <div className="flex justify-between gap-4">
                  <dt className="text-[var(--gantry-text-secondary)]">Role</dt>
                  <dd className="font-medium text-[var(--gantry-text-primary)]">{user?.effectiveRole || user?.role}</dd>
                </div>
                <div className="flex justify-between gap-4">
                  <dt className="text-[var(--gantry-text-secondary)]">Returns to</dt>
                  <dd className="truncate font-medium text-[var(--gantry-text-primary)]">{redirectHost(pending.redirectURI)}</dd>
                </div>
              </dl>
              <p className="text-xs text-[var(--gantry-text-secondary)]">
                Grant lets this client create, read, update, and link services that your role already allows. You can deny the request and nothing is shared.
              </p>
              <div className="flex gap-3">
                <button
                  type="button"
                  disabled={submitting !== ''}
                  onClick={() => decide('deny')}
                  className="flex-1 rounded-lg border border-[var(--gantry-border)] px-4 py-2.5 text-sm font-medium text-[var(--gantry-text-primary)] transition-colors hover:bg-[var(--gantry-bg-secondary)] disabled:opacity-60"
                >
                  {submitting === 'deny' ? 'Denying...' : 'Deny'}
                </button>
                <button
                  type="button"
                  disabled={submitting !== ''}
                  onClick={() => decide('grant')}
                  className="flex-1 rounded-lg bg-[var(--gantry-accent)] px-4 py-2.5 text-sm font-medium text-[var(--gantry-bg-primary)] transition-colors hover:bg-[var(--gantry-accent-hover)] disabled:opacity-60"
                >
                  {submitting === 'grant' ? 'Granting...' : 'Grant'}
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
