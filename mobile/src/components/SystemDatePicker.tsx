// The platform pickers are in SystemDatePicker.ios.tsx and .android.tsx (each
// imports only its own platform's @expo/ui views); this file gives the shared
// props and is what other platforms (web, tests) get.

export type SystemDatePickerProps = {
  part: 'date' | 'time';
  /** The picked day, "YYYY-MM-DD". */
  day: string;
  value: Date;
  max?: Date;
  onChange: (d: Date) => void;
  onClose: () => void;
};

export function SystemDatePicker(_: SystemDatePickerProps) {
  return null;
}
