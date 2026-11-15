import type { ReactNode } from 'react';
import { Pressable, StyleSheet } from 'react-native';

import { useTheme } from '@/theme';

import { Text } from './Text';

/** A selectable pill; selected is the white primary, like the web's chips. */
export function Chip({ label, selected, onPress, leading }: { label: string; selected?: boolean; onPress?: () => void; leading?: ReactNode }) {
  const { colors } = useTheme();
  return (
    <Pressable
      accessibilityRole={onPress ? 'button' : undefined}
      accessibilityState={{ selected: !!selected }}
      onPress={onPress}
      style={({ pressed }) => [
        styles.chip,
        leading ? styles.withLeading : null,
        { backgroundColor: selected ? colors.accentPrimary : colors.bgElevated, opacity: pressed ? 0.8 : 1 },
      ]}
    >
      {leading}
      <Text variant="label" style={{ color: selected ? colors.accentOn : colors.textPrimary, fontSize: 14 }}>
        {label}
      </Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  chip: { minHeight: 36, paddingHorizontal: 14, borderRadius: 18, justifyContent: 'center', flexDirection: 'row', alignItems: 'center', gap: 8 },
  withLeading: { paddingLeft: 5 },
});
