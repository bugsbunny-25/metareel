import { useQuery } from '@tanstack/react-query';
import { router, Stack } from 'expo-router';
import { clearSharedPayloads, useIncomingShare } from 'expo-sharing';
import { useEffect } from 'react';
import { ActivityIndicator, ScrollView, StyleSheet, View } from 'react-native';

import { unwrap } from '@/api/client';
import { api } from '@/auth/session';
import { Button } from '@/components/Button';
import { Poster } from '@/components/Poster';
import { Panel } from '@/components/Screen';
import { Text } from '@/components/Text';
import { useMyTitle } from '@/data/queries';
import { openLog, openTitle, setWatchlist } from '@/lib/actions';
import { year } from '@/lib/format';
import { lookupQuery, parseShared } from '@/lib/share';
import { useTheme } from '@/theme';

/**
 * Something shared to metareel from another app: a link (or text with a
 * link) is resolved to a title you can log, add to the watchlist or open;
 * anything that doesn't resolve opens search with the shared words.
 */
export default function Share() {
  const { colors, gutter } = useTheme();
  const incoming = useIncomingShare();
  const payload = incoming.sharedPayloads.find((p) => p.value?.trim());
  const shared = payload ? parseShared(payload.value) : null;
  const ref = shared && shared.type !== 'search' ? shared : null;

  const lookup = useQuery({
    queryKey: ['share-lookup', payload?.value],
    enabled: !!ref,
    retry: false,
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/titles/lookup', { params: { query: lookupQuery(ref!) }, signal })),
  });
  const found = lookup.data?.items?.[0];
  const kind = found?.kind ?? 'movie';
  const id = found?.tmdb_id ?? '';
  const title = useMyTitle(kind, id, !!found);
  const t = title.data?.title;

  // The payload is cleared once this screen goes away, not before
  // navigating: clearing first re-renders the screen as empty and the
  // navigation that follows can be lost.
  useEffect(() => () => clearSharedPayloads(), []);
  const searchFor = (q: string) => router.replace({ pathname: '/search', params: { q } });

  let body;
  if (!payload) {
    body = <Text variant="muted">Nothing was shared. Share a link from Netflix, IMDb, Letterboxd or TMDB to add or log it here.</Text>;
  } else if (shared?.type === 'search' || !shared) {
    body = (
      <>
        <Text variant="muted">This doesn&apos;t link to a title metareel can look up.</Text>
        {shared ? <Button title={`Search for “${shared.query}”`} onPress={() => searchFor(shared.query)} /> : null}
      </>
    );
  } else if (lookup.isPending || (found && title.isPending)) {
    body = <ActivityIndicator color={colors.textMuted} />;
  } else if (!found) {
    const fallback = parseShared(payload.value.replace(/https?:\/\/\S+/g, ' '));
    body = (
      <>
        <Text variant="muted">
          {lookup.isError ? "Couldn't resolve this link." : 'metareel couldn’t find this title.'}
        </Text>
        <Button kind="quiet" title="Search instead" onPress={() => searchFor(fallback?.type === 'search' ? fallback.query : '')} />
      </>
    );
  } else {
    body = (
      <>
        <View style={styles.title}>
          <Poster path={t?.poster_path} title={t?.title} width={96} />
          <View style={{ flex: 1, gap: 4 }}>
            <Text variant="title">{t?.title ?? `TMDB ${kind} ${id}`}</Text>
            <Text variant="muted">{[kind === 'tv' ? 'Series' : 'Film', year(t?.release_date)].filter(Boolean).join(' · ')}</Text>
          </View>
        </View>
        {kind === 'movie' ? (
          <Button
            title="Log it"
            onPress={() => {
              router.back();
              openLog({ kind, id, title: t?.title, poster: t?.poster_path });
            }}
          />
        ) : null}
        <Button
          kind="quiet"
          title={title.data?.state?.status === 'watchlist' ? 'On your watchlist' : 'Add to watchlist'}
          disabled={title.data?.state?.status === 'watchlist'}
          onPress={() => {
            setWatchlist({ kind, id, title: t?.title }, true);
            router.back();
          }}
        />
        <Button
          kind="quiet"
          title="Open"
          onPress={() => {
            router.back();
            openTitle({ kind, id, title: t?.title });
          }}
        />
      </>
    );
  }

  return (
    <ScrollView style={{ backgroundColor: colors.bgCanvas }} contentContainerStyle={{ padding: gutter.compact, gap: 12 }}>
      <Stack.Screen options={{ headerTitle: 'Shared with metareel' }} />
      <Panel>{body}</Panel>
      {payload ? (
        <Text variant="caption" numberOfLines={2}>
          {payload.value}
        </Text>
      ) : null}
    </ScrollView>
  );
}

const styles = StyleSheet.create({ title: { flexDirection: 'row', gap: 14, alignItems: 'center' } });
