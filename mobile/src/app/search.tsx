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
import { year } from '@/lib/format';
import { useTheme } from '@/theme';

/** A TMDB id or link the lookup endpoint resolves (the web ⌘K palette's rules). */
export function parseLookup(text: string): { tmdb?: string; imdb?: string; netflix?: string; kind?: 'movie' | 'tv' } | null {
  const t = text.trim();
  let m = t.match(/themoviedb\.org\/(movie|tv)\/(\d+)/i);
  if (m) return { kind: m[1].toLowerCase() as 'movie' | 'tv', tmdb: m[2] };
  m = t.match(/\b(tt\d{6,10})\b/i);
  if (m) return { imdb: m[1].toLowerCase() };
  m = t.match(/netflix\.com\/(?:[a-z]{2}(?:-[a-z]{2})?\/)?title\/(\d+)/i);
  if (m) return { netflix: m[1] };
  return null;
}

/** Search any title; ?mode=log opens the log sheet (or the show) on pick. */
export default function Search() {
  const { mode } = useLocalSearchParams<{ mode?: string }>();
  const { colors, fonts, gutter } = useTheme();
  const [text, setText] = useState('');
  const [q, setQ] = useState('');
  useEffect(() => {
    const t = setTimeout(() => setQ(text), 300);
    return () => clearTimeout(t);
  }, [text]);

  const ref = parseLookup(q);
  const search = useTitleSearch(ref ? '' : q);
  const lookup = useQuery({
    queryKey: ['lookup', q],
    enabled: !!ref && !ref.kind,
    queryFn: async ({ signal }) =>
      unwrap(await api.GET('/api/v1/titles/lookup', { params: { query: { imdb: ref?.imdb, netflix: ref?.netflix } }, signal })),
  });

  const pick = (kind: 'movie' | 'tv', id: string, title?: string, poster?: string) => {
    router.back();
    if (mode === 'log' && kind === 'movie') openLog({ kind, id, title, poster });
    else openTitle({ kind, id, title });
  };

  type Result = components['schemas']['TitleSearchResult'];
  const results: Result[] =
    ref?.kind && ref.tmdb
      ? [{ tmdb_kind: ref.kind, tmdb_id: ref.tmdb, title: `TMDB ${ref.kind} ${ref.tmdb}` }]
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
          placeholder="Title, or an IMDb / TMDB / Netflix link"
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
