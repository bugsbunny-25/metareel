import { useQuery } from '@tanstack/react-query';

import { unwrap } from '@/api/client';
import { api } from '@/auth/session';

/** Up next: one episode per show in progress. */
export function useUpNext(limit = 20) {
  return useQuery({
    queryKey: ['up-next', limit],
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/me/up-next', { params: { query: { limit } }, signal })),
  });
}

/** The account and its sessions (Settings). */
export function useAccount() {
  return useQuery({
    queryKey: ['account'],
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/me/account', { signal })),
  });
}
