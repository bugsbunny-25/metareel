import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useEffect, useState } from 'react';
import { StyleSheet, TextInput, View } from 'react-native';

import { openLog, openTitle } from '@/lib/actions';
import { SearchResults } from '@/screens/SearchResults';
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

  const pick = (kind: 'movie' | 'tv', id: string, title?: string, poster?: string) => {
    router.back();
    if (mode === 'log' && kind === 'movie') openLog({ kind, id, title, poster });
    else openTitle({ kind, id, title });
  };

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
      <SearchResults q={q} onPick={pick} />
    </View>
  );
}

const styles = StyleSheet.create({
  input: { minHeight: 48, borderRadius: 24, paddingHorizontal: 18, fontSize: 16 },
});
