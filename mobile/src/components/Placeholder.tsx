import { StyleSheet, View } from 'react-native';

import { useTheme } from '@/theme';

import { Text } from './Text';

/** What a detail pane shows before anything is picked. */
export function Placeholder({ text }: { text: string }) {
  const { colors } = useTheme();
  return (
    <View style={[styles.box, { backgroundColor: colors.bgCanvas }]}>
      <Text variant="muted">{text}</Text>
    </View>
  );
}

const styles = StyleSheet.create({ box: { flex: 1, alignItems: 'center', justifyContent: 'center', padding: 24 } });
