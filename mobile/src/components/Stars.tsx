import * as Haptics from 'expo-haptics';
import { Pressable, StyleSheet, View } from 'react-native';
import Svg, { Defs, LinearGradient, Path, Stop } from 'react-native-svg';

import { useTheme } from '@/theme';

const STAR = 'M12 2.5l2.9 6 6.6.9-4.8 4.6 1.2 6.5L12 17.4 6.1 20.5l1.2-6.5L2.5 9.4l6.6-.9z';

/**
 * Five stars, Letterboxd style: the value is 1..10 half-stars. Tap the left
 * half of a star for a half; tap the current value again to clear.
 */
export function Stars({ value, onChange, size = 36 }: { value: number | null; onChange?: (v: number | null) => void; size?: number }) {
  const { colors } = useTheme();
  return (
    <View style={styles.row} accessibilityRole="adjustable" accessibilityLabel="Rating" accessibilityValue={{ text: value ? `${value / 2} of 5 stars` : 'not rated' }}
      accessibilityActions={onChange ? [{ name: 'increment' }, { name: 'decrement' }] : undefined}
      onAccessibilityAction={(e) => {
        if (!onChange) return;
        const v = value ?? 0;
        onChange(e.nativeEvent.actionName === 'increment' ? Math.min(10, v + 1) : v - 1 > 0 ? v - 1 : null);
      }}
    >
      {[1, 2, 3, 4, 5].map((i) => {
        const fill = value == null ? 0 : value >= i * 2 ? 1 : value === i * 2 - 1 ? 0.5 : 0;
        return (
          <View key={i} style={{ width: size, height: size }}>
            <Svg width={size} height={size} viewBox="0 0 24 24">
              <Defs>
                <LinearGradient id={`half${i}`} x1="0" x2="1" y1="0" y2="0">
                  <Stop offset="0.5" stopColor={colors.accentYours} />
                  <Stop offset="0.5" stopColor="transparent" />
                </LinearGradient>
              </Defs>
              <Path
                d={STAR}
                fill={fill === 1 ? colors.accentYours : fill === 0.5 ? `url(#half${i})` : 'none'}
                stroke={fill ? colors.accentYours : colors.borderStrong}
                strokeWidth={1.4}
                strokeLinejoin="round"
              />
            </Svg>
            {onChange ? (
              <View style={StyleSheet.absoluteFill} accessibilityElementsHidden importantForAccessibility="no-hide-descendants">
                <View style={styles.halves}>
                  {[i * 2 - 1, i * 2].map((v) => (
                    <Pressable
                      key={v}
                      style={{ flex: 1 }}
                      hitSlop={{ top: 8, bottom: 8 }}
                      onPress={() => {
                        void Haptics.selectionAsync();
                        onChange(value === v ? null : v);
                      }}
                    />
                  ))}
                </View>
              </View>
            ) : null}
          </View>
        );
      })}
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: 'row', gap: 6 },
  halves: { flex: 1, flexDirection: 'row' },
});
