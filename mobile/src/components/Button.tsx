import { ActivityIndicator, Pressable, type PressableProps, StyleSheet } from 'react-native';

import { useTheme } from '@/theme';

import { Text } from './Text';

/** White primary action (the only coloured control), or a quiet surface button. */
export function Button({
  title,
  kind = 'primary',
  busy,
  disabled,
  ...rest
}: PressableProps & { title: string; kind?: 'primary' | 'quiet'; busy?: boolean }) {
  const { colors } = useTheme();
  const primary = kind === 'primary';
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityState={{ disabled: !!disabled || !!busy, busy: !!busy }}
      disabled={disabled || busy}
      style={({ pressed }) => [
        styles.base,
        { backgroundColor: primary ? colors.accentPrimary : colors.bgElevated, opacity: disabled ? 0.5 : pressed ? 0.85 : 1 },
      ]}
      {...rest}
    >
      {busy ? (
        <ActivityIndicator color={primary ? colors.accentOn : colors.textPrimary} />
      ) : (
        <Text variant="label" numberOfLines={1} style={{ color: primary ? colors.accentOn : colors.textPrimary, fontSize: 16, lineHeight: 21 }}>
          {title}
        </Text>
      )}
    </Pressable>
  );
}

const styles = StyleSheet.create({
  base: { minHeight: 48, borderRadius: 24, paddingHorizontal: 20, alignItems: 'center', justifyContent: 'center' },
});
