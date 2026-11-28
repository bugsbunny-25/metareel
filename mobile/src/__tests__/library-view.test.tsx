import { render, screen, userEvent } from '@testing-library/react-native';

import type { DiscCopy } from '@/discs/vocab';
import { LibraryView } from '@/screens/LibraryView';

const mockPush = jest.fn();
jest.mock('expo-router', () => ({
  router: { push: (...a: unknown[]) => mockPush(...a) },
  Stack: { Screen: () => null },
}));
jest.mock('@/auth/session', () => ({ currentSession: () => null, endpoint: () => '' }));

type Discs = { data?: { items: DiscCopy[]; totals: object }; isPending: boolean; error: Error | null; refetch: () => Promise<void> };
let mockDiscs: Discs;
jest.mock('@/discs/data', () => ({ useDiscs: () => mockDiscs }));

function copy(id: number, title: string, media_format: string, edition = ''): DiscCopy {
  return {
    id,
    retailer: '',
    release: { id: id * 10, title, edition, media_format, packaging: '', parts: [], highlights: [], disc_count: 1, images: [], titles: [], has_slipcover: false },
  } as unknown as DiscCopy;
}

function library(items: DiscCopy[]): Discs {
  const by_format: Record<string, number> = {};
  for (const c of items) by_format[c.release!.media_format] = (by_format[c.release!.media_format] ?? 0) + 1;
  return {
    data: { items, totals: { copies: items.length, discs: items.length, titles: items.length, box_sets: 0, by_format, spend: [] } },
    isPending: false,
    error: null,
    refetch: () => Promise.resolve(),
  };
}

beforeEach(() => {
  jest.useFakeTimers();
  mockPush.mockClear();
});
afterEach(() => jest.useRealTimers());

describe('LibraryView', () => {
  it('explains the empty shelf', async () => {
    mockDiscs = library([]);
    await render(<LibraryView />);
    expect(screen.getByText('Your shelf is empty')).toBeOnTheScreen();
    expect(screen.queryByRole('button', { name: 'Cases' })).not.toBeOnTheScreen();
  });

  it('pulls a spine off the shelf, then opens it on a second tap', async () => {
    mockDiscs = library([copy(1, 'Alien', 'bluray', 'Steelbook'), copy(2, 'Heat', 'dvd')]);
    const user = userEvent.setup();
    await render(<LibraryView />);

    const alien = screen.getByRole('button', { name: 'Alien, Steelbook' });
    expect(alien).toHaveProp('accessibilityHint', 'Pulls it off the shelf');
    await user.press(alien);
    expect(mockPush).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'Alien, Steelbook' })).toHaveProp('accessibilityHint', 'Opens it');

    await user.press(screen.getByRole('button', { name: 'Alien, Steelbook' }));
    expect(mockPush).toHaveBeenCalledWith({ pathname: '/disc/[id]', params: { id: '1' } });
  });

  it('shows the cases grid, filtered by format, and opens a case', async () => {
    mockDiscs = library([copy(1, 'Alien', 'bluray'), copy(2, 'Heat', 'dvd'), copy(3, 'Ran', 'bluray')]);
    const user = userEvent.setup();
    await render(<LibraryView />);

    await user.press(screen.getByRole('button', { name: 'Cases' }));
    expect(screen.getByRole('button', { name: 'Cases' })).toBeSelected();
    // Each case shows its title on the cover and under it.
    expect(screen.getAllByText('Heat')).not.toHaveLength(0);

    await user.press(screen.getByRole('button', { name: 'Blu-ray' }));
    expect(screen.queryAllByText('Heat')).toHaveLength(0);
    await user.press(screen.getAllByText('Ran')[0]);
    expect(mockPush).toHaveBeenCalledWith({ pathname: '/disc/[id]', params: { id: '3' } });
  });
});
