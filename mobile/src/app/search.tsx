import { useQuery } from '@tanstack/react-query';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useEffect, useState } from 'react';
import { ActivityIndicator, FlatList, StyleSheet, TextInput, View } from 'react-native';

import { unwrap } from '@/api/client';
import type { components } from '@/api/schema';
import { api } from '@/auth/session';
import { Text } from '@/components/Text';
import { TitleRow } from '@/components/TitleRow';
import { useTitleSearch } from '@/data/queries';
import { openLog, openTitle } from '@/lib/actions';
import { lookupQuery, parseShared } from '@/lib/share';
import { year } from '@/lib/format';
import { useTheme } from '@/theme';

/** Search any title; ?mode=log opens the log sheet (or the show) on pick. */
export default function Search() {
  const { mode, q: initial } = useLocalSearchParams<{ mode?: string; q?: string }>();
  const { colors, fonts, gutter } = useTheme();
  const [text, setText] = useState(initial ?? '');
  const [q, setQ] = useState(initial ?? '');
  useEffect(() => {
    const t = setTimeout(() => setQ(text), 300);
    return () => clearTimeout(t);
  }, [text]);

  // A pasted link or ID is looked up; anything else is a title search.
  const parsed = parseShared(q);
  const ref = parsed && parsed.type !== 'search' ? parsed : null;
  const search = useTitleSearch(ref ? '' : q);
  const lookup = useQuery({
    queryKey: ['lookup', q],
    enabled: !!ref && ref.type !== 'tmdb',
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/titles/lookup', { params: { query: lookupQuery(ref!) }, signal })),
  });

  const pick = (kind: 'movie' | 'tv', id: string, title?: string, poster?: string) => {
    router.back();
    if (mode === 'log' && kind === 'movie') openLog({ kind, id, title, poster });
    else openTitle({ kind, id, title });
  };

  type Result = components['schemas']['TitleSearchResult'];
  const results: Result[] =
    ref?.type === 'tmdb'
      ? [{ tmdb_kind: ref.kind, tmdb_id: ref.id, title: `TMDB ${ref.kind} ${ref.id}` }]
      : ref
        ? (lookup.data?.items ?? []).map((i) => ({ tmdb_kind: i.kind, tmdb_id: i.tmdb_id, title: `Found: ${i.kind} ${i.tmdb_id}` }))
        : (search.data?.results ?? []);
  const busy = search.isFetching || lookup.isFetching;

  return (
    <View style={{ flex: 1, backgroundColor: colors.bgCanvas }}>
      <Stack.Screen options={{ headerTitle: mode === 'log' ? 'Log a film' : 'Search' }} />
      <View style={{ padding: gutter.compact, paddingBottom: 8 }}>
        <TextInput
          autoFocus
          value={text}
          onChangeText={setText}
          placeholder="Title, or an IMDb / TMDB / Netflix / Letterboxd link"
          placeholderTextColor={colors.textMuted}
          returnKeyType="search"
          autoCorrect={false}
          style={[styles.input, { backgroundColor: colors.bgElevated, color: colors.textPrimary, fontFamily: fonts.sans }]}
          accessibilityLabel="Search titles"
        />
      </View>
      {busy ? <ActivityIndicator color={colors.textMuted} /> : null}
      {search.isError ? <Text variant="muted" style={{ paddingHorizontal: gutter.compact }}>Search needs a connection.</Text> : null}
      <FlatList
        data={results}
        keyExtractor={(r) => `${r.tmdb_kind}-${r.tmdb_id}`}
        keyboardShouldPersistTaps="handled"
        contentContainerStyle={{ paddingHorizontal: gutter.compact, paddingBottom: 32 }}
        renderItem={({ item: r }) => {
          const poster = r.poster_path;
          const release = r.release_date;
          return (
            <TitleRow
              poster={poster}
              title={r.title ?? ''}
              subtitle={[r.tmdb_kind === 'tv' ? 'Series' : 'Film', year(release)].filter(Boolean).join(' · ')}
              onPress={() => pick(r.tmdb_kind ?? 'movie', r.tmdb_id ?? '', r.title, poster)}
            />
          );
        }}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  input: { minHeight: 48, borderRadius: 24, paddingHorizontal: 18, fontSize: 16 },
});
