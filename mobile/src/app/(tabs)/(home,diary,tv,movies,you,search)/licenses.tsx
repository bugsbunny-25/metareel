import { Stack } from 'expo-router';
import { FlatList, Linking, Pressable, StyleSheet, View } from 'react-native';

import { Icon } from '@/components/Icon';
import { Text } from '@/components/Text';
import licenses from '@/data/licenses.json';
import { useTheme } from '@/theme';

type Item = { name: string; version?: string; license: string; url: string };

/**
 * Open source licenses (from Settings): the typefaces and every package the
 * app ships, with its license; a row opens the project. The list is
 * generated from node_modules (npm run gen:licenses).
 */
export default function Licenses() {
  const { colors, gutter } = useTheme();
  const items: Item[] = [...licenses.fonts, ...licenses.packages];
  return (
    <View style={{ flex: 1, backgroundColor: colors.bgCanvas }}>
      <Stack.Screen options={{ headerTitle: 'Open source licenses' }} />
      <FlatList
        data={items}
        keyExtractor={(i) => `${i.name}@${i.version ?? ''}`}
        contentInsetAdjustmentBehavior="automatic"
        initialNumToRender={30}
        contentContainerStyle={{ padding: gutter.compact, paddingBottom: 40, width: '100%', maxWidth: 760, alignSelf: 'center' }}
        ListHeaderComponent={
          <Text variant="muted" style={{ marginBottom: 12 }}>
            metareel is built on {licenses.packages.length} open source packages and {licenses.fonts.length} typefaces. Tap one to open its project and full
            license.
          </Text>
        }
        renderItem={({ item }) => (
          <Pressable
            onPress={() => void Linking.openURL(item.url)}
            accessibilityRole="link"
            accessibilityLabel={`${item.name}${item.version ? ` ${item.version}` : ''}, ${item.license} license`}
            style={({ pressed }) => [styles.row, { borderBottomColor: colors.borderSubtle, opacity: pressed ? 0.6 : 1 }]}
          >
            <View style={{ flex: 1, gap: 2 }}>
              <Text variant="label" numberOfLines={1}>
                {item.name}
              </Text>
              <Text variant="caption" num>
                {item.version ? `${item.version} · ` : ''}
                {item.license}
              </Text>
            </View>
            <Icon name="chevron" size={16} color={colors.textMuted} />
          </Pressable>
        )}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: 'row', alignItems: 'center', gap: 10, paddingVertical: 10, borderBottomWidth: StyleSheet.hairlineWidth },
});
