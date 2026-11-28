// The platform toggles are in Toggle.ios.tsx and Toggle.android.tsx; this file
// gives the shared props and is what other platforms (web, tests) get.
import { Host, Switch } from '@expo/ui';

import { useTheme } from '@/theme';

export type ToggleProps = {
  value: boolean;
  onValueChange: (v: boolean) => void;
  /** What assistive tech announces; the row shows its own visible text. */
  label: string;
  disabled?: boolean;
};

export function Toggle({ value, onValueChange, label, disabled }: ToggleProps) {
  const { colors, scheme } = useTheme();
  return (
    <Host matchContents colorScheme={scheme} seedColor={colors.accentYours}>
      <Switch value={value} onValueChange={onValueChange} label={label} disabled={disabled} />
    </Host>
  );
}
