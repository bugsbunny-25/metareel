import { StyleSheet, View } from 'react-native';

/**
 * The fill behind a selected pill (a focused tab, a picked segment). Its
 * colour never changes, only its opacity: on Android, changing a rounded
 * view's backgroundColor after mount redraws it without the radius, as a
 * square. Put it first inside the pill.
 */
export function PillFill({ on, color, radius }: { on: boolean; color: string; radius: number }) {
  return <View pointerEvents="none" style={[StyleSheet.absoluteFill, { borderRadius: radius, backgroundColor: color, opacity: on ? 1 : 0 }]} />;
}
