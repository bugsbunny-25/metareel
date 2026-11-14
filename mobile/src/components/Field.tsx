import { StyleSheet, TextInput, type TextInputProps, View } from 'react-native';

import { useTheme } from '@/theme';

import { Text } from './Text';

export function Field({ label, hint, style, ...rest }: TextInputProps & { label: string; hint?: string }) {
  const { colors, fonts } = useTheme();
  return (
    <View style={styles.wrap}>
      <Text variant="caption">{label}</Text>
      <TextInput
        placeholderTextColor={colors.textMuted}
        selectionColor={colors.accentPrimary}
        style={[
          styles.input,
          { backgroundColor: colors.bgElevated, color: colors.textPrimary, fontFamily: fonts.sans, borderColor: colors.borderSubtle },
          style,
        ]}
        {...rest}
      />
      {hint ? <Text variant="muted" style={{ fontSize: 13 }}>{hint}</Text> : null}
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: { gap: 6 },
  input: { minHeight: 48, borderRadius: 12, paddingHorizontal: 14, fontSize: 16, borderWidth: StyleSheet.hairlineWidth },
});
