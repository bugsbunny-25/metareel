import type { ReactNode } from 'react';
import { Pressable, StyleSheet, View } from 'react-native';

import { useTheme } from '@/theme';

import { Poster } from './Poster';
import { Text } from './Text';

/** A poster, a title and a line under it; the whole row is one button. */
export function TitleRow({
  poster,
  title,
  subtitle,
  right,
  onPress,
  accent,
  selected,
  kind,
  tmdbId,
}: {
  poster?: string | null;
  title: string;
  subtitle?: ReactNode;
  right?: ReactNode;
  onPress?: () => void;
  accent?: string;
  /** The row shown in the detail pane beside the list. */
  selected?: boolean;
  /** With tmdbId: the Top 10 badge on the poster when the title charts today. */
  kind?: string | null;
  tmdbId?: string | null;
}) {
  const { colors } = useTheme();
  return (
    <Pressable
      onPress={onPress}
      disabled={!onPress}
      accessibilityRole={onPress ? 'button' : undefined}
      accessibilityState={selected ? { selected: true } : undefined}
      style={({ pressed }) => [styles.row, selected && { backgroundColor: colors.bgElevated, marginHorizontal: -8, paddingHorizontal: 8, borderRadius: 12, borderCurve: 'continuous' }, { opacity: pressed ? 0.75 : 1 }]}
    >
      <Poster path={poster} title={title} width={44} kind={kind} tmdbId={tmdbId} />
      <View style={styles.text}>
        <Text variant="label" numberOfLines={2} style={accent ? { color: accent } : undefined}>
          {title}
        </Text>
        {typeof subtitle === 'string' ? (
          <Text variant="muted" numberOfLines={2}>
            {subtitle}
          </Text>
        ) : (
          subtitle
        )}
      </View>
      {right}
    </Pressable>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: 'row', alignItems: 'center', gap: 12, paddingVertical: 6 },
  text: { flex: 1, gap: 2 },
});
