import type { ReactNode } from 'react';
import { Pressable, StyleSheet, View } from 'react-native';

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
}: {
  poster?: string | null;
  title: string;
  subtitle?: ReactNode;
  right?: ReactNode;
  onPress?: () => void;
  accent?: string;
}) {
  return (
    <Pressable
      onPress={onPress}
      disabled={!onPress}
      accessibilityRole={onPress ? 'button' : undefined}
      style={({ pressed }) => [styles.row, { opacity: pressed ? 0.75 : 1 }]}
    >
      <Poster path={poster} title={title} width={44} />
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
