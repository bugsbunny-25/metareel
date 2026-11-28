import { Image } from 'expo-image';
import { StyleSheet, View } from 'react-native';

import { useTop10Of } from '@/data/top10now';
import { tmdbImage } from '@/lib/format';
import { useTheme } from '@/theme';

import { Text } from './Text';
import { Top10Badge } from './Top10Badge';

/**
 * A 2:3 poster with the hairline the web uses; initials when there is none.
 * kind + tmdbId put the Top 10 badge on a title charting in your country
 * today (chart screens leave them out: the chart says so already).
 */
export function Poster({
  path,
  title,
  width = 56,
  kind,
  tmdbId,
}: {
  path?: string | null;
  title?: string;
  width?: number;
  kind?: string | null;
  tmdbId?: string | null;
}) {
  const { colors } = useTheme();
  const top = useTop10Of(kind, tmdbId);
  const uri = tmdbImage(path, width > 120 ? 'w342' : 'w185');
  const box = { width, height: Math.round(width * 1.5), borderRadius: width > 100 ? 12 : 8, borderCurve: 'continuous' as const };
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
      {top ? (
        <View style={[styles.badge, width < 60 && { top: 2, right: 2 }]}>
          <Top10Badge size={width < 60 ? 16 : width < 110 ? 22 : 28} />
        </View>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  badge: { position: 'absolute', top: 5, right: 5 },
  wrap: { overflow: 'hidden', alignItems: 'center', justifyContent: 'center', borderWidth: StyleSheet.hairlineWidth },
});
