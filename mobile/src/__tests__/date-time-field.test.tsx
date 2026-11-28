import { act, render, screen, userEvent } from '@testing-library/react-native';

import { DateTimeField } from '@/components/DateTimeField';
import type { SystemDatePickerProps } from '@/components/SystemDatePicker';

// The native pickers (@expo/ui SwiftUI / Compose) stand in as a line of text;
// the test drives their callbacks the way a pick or a dismissal would.
let mockPicker: SystemDatePickerProps | null = null;
jest.mock('@/components/SystemDatePicker', () => ({
  SystemDatePicker: (p: SystemDatePickerProps) => {
    const { Text } = jest.requireActual<typeof import('react-native')>('react-native');
    mockPicker = p;
    return <Text>{`picker: ${p.part} ${p.day}`}</Text>;
  },
}));

beforeEach(() => {
  jest.useFakeTimers();
  mockPicker = null;
});
afterEach(() => jest.useRealTimers());

describe('DateTimeField', () => {
  it('opens the date picker from the day chip and keeps the time when a day is picked', async () => {
    const onChange = jest.fn();
    const user = userEvent.setup();
    await render(<DateTimeField value="2026-11-28T20:15" onChange={onChange} />);
    expect(screen.queryByText(/^picker:/)).not.toBeOnTheScreen();

    const [dayChip, timeChip] = screen.getAllByRole('button');
    await user.press(dayChip);
    expect(dayChip).toBeSelected();
    expect(timeChip).not.toBeSelected();
    expect(screen.getByText('picker: date 2026-11-28')).toBeOnTheScreen();

    await act(() => mockPicker!.onChange(new Date(2026, 10, 3, 12, 0)));
    expect(onChange).toHaveBeenLastCalledWith('2026-11-03T20:15');
  });

  it('changes only the time from the time picker, and closes on dismiss', async () => {
    const onChange = jest.fn();
    const user = userEvent.setup();
    await render(<DateTimeField value="2026-11-28T20:15" onChange={onChange} />);

    await user.press(screen.getAllByRole('button')[1]);
    expect(screen.getByText('picker: time 2026-11-28')).toBeOnTheScreen();
    await act(() => mockPicker!.onChange(new Date(2026, 0, 1, 9, 30)));
    expect(onChange).toHaveBeenLastCalledWith('2026-11-28T09:30');

    await act(() => mockPicker!.onClose());
    expect(screen.queryByText(/^picker:/)).not.toBeOnTheScreen();
  });

  it('is a single day chip without a time', async () => {
    const onChange = jest.fn();
    const user = userEvent.setup();
    await render(<DateTimeField value="2026-11-28" withTime={false} onChange={onChange} />);

    expect(screen.getAllByRole('button')).toHaveLength(1);
    await user.press(screen.getByRole('button'));
    await act(() => mockPicker!.onChange(new Date(2026, 10, 3, 12, 0)));
    expect(onChange).toHaveBeenLastCalledWith('2026-11-03');
  });

  it('closes the open picker when its chip is pressed again', async () => {
    const user = userEvent.setup();
    await render(<DateTimeField value="2026-11-28T20:15" onChange={jest.fn()} />);
    const [dayChip] = screen.getAllByRole('button');
    await user.press(dayChip);
    await user.press(dayChip);
    expect(screen.queryByText(/^picker:/)).not.toBeOnTheScreen();
  });
});
