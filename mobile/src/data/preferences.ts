import { unwrap } from '@/api/client';
import { api } from '@/auth/session';
import { queryClient } from '@/query/client';

/** Replaces some preferences, keeping the rest (PUT replaces the whole set). */
export async function updatePreferences(patch: Record<string, unknown>) {
  const current = unwrap(await api.GET('/api/v1/me/preferences')).preferences ?? {};
  const saved = unwrap(await api.PUT('/api/v1/me/preferences', { body: { ...current, ...patch } }));
  queryClient.setQueryData(['preferences'], saved);
  return saved.preferences ?? {};
}
