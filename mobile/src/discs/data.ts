import { useQuery } from '@tanstack/react-query';

import { apiImageRequest, unwrap } from '@/api/client';
import type { components } from '@/api/schema';
import { api, apiFetch } from '@/auth/session';
import { queryClient } from '@/query/client';

import type { ImageKind } from './vocab';

type AddDiscInput = components['schemas']['AddDiscInput'];
type DiscCopyInput = components['schemas']['DiscCopyInput'];

/** Your collection: every copy with its release, and totals. */
export function useDiscs() {
  return useQuery({
    queryKey: ['discs'],
    queryFn: async ({ signal }) =>
      unwrap(
        await api.GET('/api/v1/me/discs', {
          params: { query: { sort: 'added' } },
          signal,
        }),
      ),
  });
}

/** One of your copies with its full release. */
export function useDisc(id: number) {
  return useQuery({
    queryKey: ['disc', id],
    queryFn: async ({ signal }) =>
      unwrap(
        await api.GET('/api/v1/me/discs/{id}', {
          params: { path: { id } },
          signal,
        }),
      ),
  });
}

/** What metareel (or the barcode service) knows about a scanned code. */
export async function lookupDisc(barcode: string) {
  return unwrap(
    await api.GET('/api/v1/me/discs/lookup', {
      params: { query: { barcode } },
    }),
  );
}

const refresh = () =>
  void queryClient.invalidateQueries({
    predicate: (q) => q.queryKey[0] === 'discs' || q.queryKey[0] === 'disc',
  });

export async function addDisc(body: AddDiscInput) {
  const out = unwrap(await api.POST('/api/v1/me/discs', { body }));
  refresh();
  return out;
}

export async function updateDisc(id: number, body: DiscCopyInput) {
  const out = unwrap(await api.PUT('/api/v1/me/discs/{id}', { params: { path: { id } }, body }));
  refresh();
  return out;
}

export async function deleteDisc(id: number) {
  const r = await api.DELETE('/api/v1/me/discs/{id}', {
    params: { path: { id } },
  });
  if (!r.response.ok) throw new Error(`Couldn't remove it (HTTP ${r.response.status})`);
  refresh();
}

/** Uploads a photo's bytes as a picture of a release (or one of its parts). */
export async function uploadDiscImage(releaseId: number, kind: ImageKind, bytes: ArrayBuffer, contentType: string) {
  const res = await apiFetch(apiImageRequest(`/api/v1/me/disc-releases/${releaseId}/images?kind=${kind}`, bytes, contentType));
  if (!res.ok) {
    let msg = `HTTP ${res.status}`;
    try {
      msg = ((await res.json()) as { error?: string }).error ?? msg;
    } catch {
      // not JSON
    }
    throw new Error(msg);
  }
  refresh();
}

export async function deleteDiscImage(releaseId: number, imageId: number) {
  const r = await api.DELETE('/api/v1/me/disc-releases/{id}/images/{image_id}', { params: { path: { id: releaseId, image_id: imageId } } });
  if (!r.response.ok) throw new Error(`Couldn't delete the picture (HTTP ${r.response.status})`);
  refresh();
}

export async function searchTitles(q: string) {
  return unwrap(
    await api.GET('/api/v1/me/titles/search', {
      params: { query: { q, limit: 8 } },
    }),
  );
}
