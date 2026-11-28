import { Host, Switch } from '@expo/ui';
import { labelsHidden } from '@expo/ui/swift-ui/modifiers';

import { useTheme } from '@/theme';

import type { ToggleProps } from './Toggle';

/**
 * A native SwiftUI toggle in the app's accent. The label is hidden (the row
 * shows its own text) but still read by VoiceOver.
 */
export function Toggle({ value, onValueChange, label, disabled }: ToggleProps) {
  const { colors, scheme } = useTheme();
  return (
    <Host matchContents colorScheme={scheme} seedColor={colors.accentYours}>
      <Switch value={value} onValueChange={onValueChange} label={label} disabled={disabled} modifiers={[labelsHidden()]} />
    </Host>
  );
}
