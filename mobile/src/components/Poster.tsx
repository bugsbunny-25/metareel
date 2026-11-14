import { Image } from 'expo-image';
import { StyleSheet, View } from 'react-native';

import { tmdbImage } from '@/lib/format';
import { useTheme } from '@/theme';

import { Text } from './Text';

/** A 2:3 poster with the hairline the web uses; initials when there is none. */
export function Poster({ path, title, width = 56 }: { path?: string | null; title?: string; width?: number }) {
  const { colors } = useTheme();
  const uri = tmdbImage(path, width > 120 ? 'w342' : 'w185');
  const box = { width, height: Math.round(width * 1.5), borderRadius: width > 100 ? 12 : 8 };
  return (
    <View style={[box, styles.wrap, { backgroundColor: colors.bgElevated, borderColor: colors.posterHairline }]}>
      {uri ? (
        <Image source={{ uri }} style={StyleSheet.absoluteFill} contentFit="cover" transition={150} accessible={false} />
      ) : (
        <Text variant="caption" style={{ textAlign: 'center' }}>
          {(title ?? '')
            .split(/\s+/)
            .slice(0, 2)
            .map((w) => w[0])
            .join('')}
        </Text>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: { overflow: 'hidden', alignItems: 'center', justifyContent: 'center', borderWidth: StyleSheet.hairlineWidth },
});
