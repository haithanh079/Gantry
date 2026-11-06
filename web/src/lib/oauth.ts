// safeOAuthNext allows only same-origin paths under /oauth/.
export function safeOAuthNext(raw: string | null | undefined): string {
  if (!raw) return '';
  if (!raw.startsWith('/oauth/') || raw.startsWith('//') || raw.includes('\\') || raw.includes('://')) {
    return '';
  }
  try {
    const url = new URL(raw, window.location.origin);
    if (url.origin !== window.location.origin || !url.pathname.startsWith('/oauth/')) {
      return '';
    }
    return url.pathname + url.search;
  } catch {
    return '';
  }
}

export const OAUTH_NEXT_STORAGE_KEY = 'gantry_oauth_next';
