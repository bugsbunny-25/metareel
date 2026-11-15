import { Stack } from 'expo-router';
import { useEffect, useState } from 'react';
import { View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { openTitle } from '@/lib/actions';
import { SearchResults } from '@/screens/SearchResults';
import { useTheme } from '@/theme';

/**
 * iOS's search tab: the system search field (in the tab bar on iPhone,
 * beside the tabs in the iPhone Duo's control strip and on iPad), with the
 * results under it. A pick opens in this tab's own stack. Android searches
 * from the button beside each tab's title instead (app/search.tsx).
 */
export default function Find() {
  const { colors } = useTheme();
  const insets = useSafeAreaInsets();
  const [text, setText] = useState('');
  const [q, setQ] = useState('');
  useEffect(() => {
    const t = setTimeout(() => setQ(text), 300);
    return () => clearTimeout(t);
  }, [text]);
  return (
    <View style={{ flex: 1, backgroundColor: colors.bgCanvas }}>
      <Stack.SearchBar
        placement="automatic"
        placeholder="Title, or an IMDb / TMDB / Letterboxd link"
        autoCapitalize="none"
        tintColor={colors.accentYours}
        textColor={colors.textPrimary}
        onChangeText={(e) => setText(e.nativeEvent.text)}
        onCancelButtonPress={() => setText('')}
      />
      <SearchResults
        q={q}
        onPick={(kind, id, title) => openTitle({ kind, id, title })}
        contentInsetAdjustmentBehavior="automatic"
        insetLeft={insets.left}
        insetRight={insets.right}
        empty="Search films and shows, or paste a link from IMDb, TMDB, Netflix or Letterboxd."
      />
    </View>
  );
}
