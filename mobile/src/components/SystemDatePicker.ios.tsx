import { Host } from '@expo/ui';
import { DatePicker } from '@expo/ui/swift-ui';
import { datePickerStyle, labelsHidden, tint } from '@expo/ui/swift-ui/modifiers';
import { StyleSheet, View } from 'react-native';

import { useTheme } from '@/theme';

import type { SystemDatePickerProps } from './SystemDatePicker';

/**
 * The system calendar (date) or time wheel, inline under the chips, tinted
 * like the app. A picked day closes the calendar; the time wheel stays open
 * while it's spun.
 */
export function SystemDatePicker({ part, value, max, onChange, onClose }: SystemDatePickerProps) {
  const { colors, scheme, radius } = useTheme();
  return (
    <View style={[styles.panel, { backgroundColor: colors.bgElevated, borderRadius: radius.panel, borderCurve: 'continuous' }]}>
      <Host matchContents={{ vertical: true }} colorScheme={scheme} style={{ width: '100%' }}>
        <DatePicker
          selection={value}
          range={max ? { end: max } : undefined}
          displayedComponents={part === 'date' ? ['date'] : ['hourAndMinute']}
          onDateChange={(d) => {
            onChange(d);
            if (part === 'date') onClose();
          }}
          modifiers={[datePickerStyle(part === 'date' ? 'graphical' : 'wheel'), tint(colors.accentYours), labelsHidden()]}
        />
      </Host>
    </View>
  );
}

const styles = StyleSheet.create({
  panel: { padding: 8, overflow: 'hidden', borderCurve: 'continuous' },
});
