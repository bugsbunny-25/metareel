import { useState } from 'react';
import { Platform, StyleSheet, View } from 'react-native';

import { pickerToWall, wallToPicker } from '@/log/zones';
import { useTheme } from '@/theme';

import { Chip } from './Chip';

/**
 * A date (and time) as a wall-clock string ("YYYY-MM-DDTHH:MM", or
 * "YYYY-MM-DD" for a date), shown as two chips like the rest of the sheet.
 * Tapping one opens the system picker: on iOS a calendar or a time wheel
 * inline under the chips, on Android the Material dialogs. The value is wall
 * time in the watch's zone, shown as the same digits whatever the device's
 * zone.
 */
export function DateTimeField({ value, onChange, withTime = true, max }: { value: string; onChange: (v: string) => void; withTime?: boolean; max?: Date }) {
  const [open, setOpen] = useState<'date' | 'time' | null>(null);
  const day = value.slice(0, 10);
  const time = withTime ? value.slice(11, 16) : '12:00';
  const local = wallToPicker(`${day}T${time}`);
  const dayText = local.toLocaleDateString(undefined, { weekday: 'short', day: 'numeric', month: 'short', year: 'numeric' });
  const timeText = local.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' });
  const set = (d: Date, part: 'date' | 'time') => {
    const w = pickerToWall(d);
    if (!withTime) onChange(w.slice(0, 10));
    else onChange(part === 'date' ? `${w.slice(0, 10)}T${time}` : `${day}T${w.slice(11, 16)}`);
  };
  const toggle = (p: 'date' | 'time') => setOpen((o) => (o === p ? null : p));
  return (
    <View style={{ gap: 10 }}>
      <View style={styles.row}>
        <Chip label={dayText} selected={open === 'date'} onPress={() => toggle('date')} />
        {withTime ? <Chip label={timeText} selected={open === 'time'} onPress={() => toggle('time')} /> : null}
      </View>
      {open && Platform.OS === 'ios' ? (
        <IOSPicker
          part={open}
          value={local}
          max={max}
          onChange={(d) => {
            set(d, open);
            // A picked day closes the calendar; the time wheel stays while it's spun.
            if (open === 'date') setOpen(null);
          }}
        />
      ) : null}
      {open && Platform.OS === 'android' ? (
        <AndroidDialog
          part={open}
          day={day}
          value={local}
          max={max}
          onClose={() => setOpen(null)}
          onChange={(d) => {
            setOpen(null);
            set(d, open);
          }}
        />
      ) : null}
    </View>
  );
}

/** The system calendar (date) or time wheel, inline, tinted like the app. */
function IOSPicker({ part, value, max, onChange }: { part: 'date' | 'time'; value: Date; max?: Date; onChange: (d: Date) => void }) {
  const { colors, scheme, radius } = useTheme();
  // Loaded here so Android never requires the SwiftUI views.
  const { DatePicker, Host } = require('@expo/ui/swift-ui') as typeof import('@expo/ui/swift-ui');
  const { datePickerStyle, tint, labelsHidden } = require('@expo/ui/swift-ui/modifiers') as typeof import('@expo/ui/swift-ui/modifiers');
  return (
    <View style={[styles.panel, { backgroundColor: colors.bgElevated, borderRadius: radius.panel }]}>
      <Host matchContents={{ vertical: true }} colorScheme={scheme} style={{ width: '100%' }}>
        <DatePicker
          selection={value}
          range={max ? { end: max } : undefined}
          displayedComponents={part === 'date' ? ['date'] : ['hourAndMinute']}
          onDateChange={onChange}
          modifiers={[datePickerStyle(part === 'date' ? 'graphical' : 'wheel'), tint(colors.accentYours), labelsHidden()]}
        />
      </Host>
    </View>
  );
}

function AndroidDialog({
  part,
  day,
  value,
  max,
  onClose,
  onChange,
}: {
  part: 'date' | 'time';
  day: string;
  value: Date;
  max?: Date;
  onClose: () => void;
  onChange: (d: Date) => void;
}) {
  const { DatePickerDialog, TimePickerDialog, Host } = require('@expo/ui/jetpack-compose') as typeof import('@expo/ui/jetpack-compose');
  return (
    <Host matchContents>
      {part === 'date' ? (
        // The Material date picker works in UTC days; hand back a local date.
        <DatePickerDialog
          initialDate={`${day}T00:00:00Z`}
          selectableDates={max ? { end: max } : undefined}
          onDismissRequest={onClose}
          onDateSelected={(d) => onChange(new Date(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate(), value.getHours(), value.getMinutes()))}
        />
      ) : (
        <TimePickerDialog initialDate={value.toISOString()} is24Hour={false} onDismissRequest={onClose} onDateSelected={onChange} />
      )}
    </Host>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: 'row', flexWrap: 'wrap', gap: 8 },
  panel: { padding: 8, overflow: 'hidden' },
});
