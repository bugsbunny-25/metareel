import { Image } from 'expo-image';
import { StyleSheet } from 'react-native';

import { tmdbImage } from '@/lib/format';
import { useTheme } from '@/theme';

/** A streaming service's TMDB logo, a rounded square (chips, rows). */
export function ServiceLogo({ path, size = 26 }: { path?: string | null; size?: number }) {
  const { colors } = useTheme();
  const uri = tmdbImage(path, 'w92');
  if (!uri) return null;
  return (
    <Image
      source={{ uri }}
      style={{ width: size, height: size, borderRadius: Math.round(size / 4), borderCurve: 'continuous', borderWidth: StyleSheet.hairlineWidth, borderColor: colors.posterHairline }}
      accessible={false}
    />
  );
}
