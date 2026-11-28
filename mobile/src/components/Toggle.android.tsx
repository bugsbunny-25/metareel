import { Switch } from 'react-native';

import { useTheme } from '@/theme';

import type { ToggleProps } from './Toggle';

/**
 * React Native's switch on Android, not @expo/ui's: the Compose switch
 * exposes its own unlabelled switch node to TalkBack, and the row shows its
 * text beside it, so it can't carry a visible label either. This one is a
 * single labelled switch node.
 */
export function Toggle({ value, onValueChange, label, disabled }: ToggleProps) {
  const { colors } = useTheme();
  return (
    <Switch
      value={value}
      onValueChange={onValueChange}
      disabled={disabled}
      accessibilityLabel={label}
      trackColor={{ true: colors.accentYours, false: colors.borderStrong }}
      // A white thumb on the accent track, as the app always had (a one-off, not a theme colour).
      thumbColor="#ffffff"
    />
  );
}
