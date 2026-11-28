import { Host } from '@expo/ui';
import { DatePickerDialog, TimePickerDialog } from '@expo/ui/jetpack-compose';

import type { SystemDatePickerProps } from './SystemDatePicker';

/** The Material date or time dialog; picking closes it. */
export function SystemDatePicker({ part, day, value, max, onChange, onClose }: SystemDatePickerProps) {
  const pick = (d: Date) => {
    onClose();
    onChange(d);
  };
  return (
    <Host matchContents>
      {part === 'date' ? (
        // The Material date picker works in UTC days; hand back a local date.
        <DatePickerDialog
          initialDate={`${day}T00:00:00Z`}
          selectableDates={max ? { end: max } : undefined}
          onDismissRequest={onClose}
          onDateSelected={(d) => pick(new Date(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate(), value.getHours(), value.getMinutes()))}
        />
      ) : (
        <TimePickerDialog initialDate={value.toISOString()} is24Hour={false} onDismissRequest={onClose} onDateSelected={pick} />
      )}
    </Host>
  );
}
