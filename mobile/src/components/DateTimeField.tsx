import { useState } from 'react';
import { StyleSheet, View } from 'react-native';

import { pickerToWall, wallToPicker } from '@/log/zones';

import { Chip } from './Chip';
import { SystemDatePicker } from './SystemDatePicker';

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
      {open ? <SystemDatePicker part={open} day={day} value={local} max={max} onChange={(d) => set(d, open)} onClose={() => setOpen(null)} /> : null}
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: 'row', flexWrap: 'wrap', gap: 8 },
});
