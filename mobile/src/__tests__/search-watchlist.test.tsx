import { render, screen, userEvent } from '@testing-library/react-native';

import type { OutboxItem } from '@/offline/outbox';
import { SearchResults } from '@/screens/SearchResults';

const mockSetWatchlist = jest.fn();
let mockQueued: Partial<OutboxItem>[] = [];
jest.mock('@/lib/actions', () => ({ setWatchlist: (...a: unknown[]) => mockSetWatchlist(...a) }));
jest.mock('expo-haptics', () => ({ selectionAsync: () => Promise.resolve() }));
jest.mock('@/auth/session', () => ({ api: { GET: jest.fn() }, currentSession: () => null, endpoint: () => '' }));
jest.mock('@/screens/DiscoverRails', () => ({ DiscoverRails: () => null }));
jest.mock('@/data/queries', () => ({
  useTitleSearch: () => ({
    data: {
      results: [
        { tmdb_kind: 'movie', tmdb_id: '1', title: 'Alien', release_date: '1979-05-25' },
        { tmdb_kind: 'tv', tmdb_id: '2', title: 'Severance' },
        { tmdb_kind: 'movie', tmdb_id: '3', title: 'Heat' },
      ],
    },
    isFetching: false,
    isError: false,
  }),
  useWatchlist: () => ({ data: { titles: [{ tmdb_kind: 'tv', tmdb_id: '2', status: 'watchlist' }] } }),
  useMyTitle: (kind: string, id: string) =>
    kind === 'movie' && id === '1518204' ? { data: { title: { title: 'Allen', poster_path: '/allen.jpg', release_date: '2026-11-13' } } } : { data: undefined },
}));
jest.mock('@/offline', () => ({
  watchlistShown: jest.requireActual<typeof import('@/offline/watchlist')>('@/offline/watchlist').watchlistShown,
  useOutbox: () => mockQueued,
}));
jest.mock('@tanstack/react-query', () => ({ useQuery: () => ({ data: undefined, isFetching: false }) }));
jest.mock('@/components/TitleRow', () => {
  const { View, Text } = jest.requireActual<typeof import('react-native')>('react-native');
  return {
    TitleRow: ({ title, right }: { title: string; right?: React.ReactNode }) => (
      <View>
        <Text>{title}</Text>
        {right}
      </View>
    ),
  };
});

beforeEach(() => {
  jest.useFakeTimers();
  mockSetWatchlist.mockClear();
  mockQueued = [];
});
afterEach(() => jest.useRealTimers());

describe('search results watchlist button', () => {
  it('shows which results are already on the watchlist', async () => {
    await render(<SearchResults q="a" onPick={jest.fn()} />);
    expect(screen.getByRole('button', { name: 'Add Alien to your watchlist' })).not.toBeSelected();
    expect(screen.getByRole('button', { name: 'Remove Severance from your watchlist' })).toBeSelected();
  });

  it('adds or removes a title without opening it', async () => {
    const user = userEvent.setup();
    const onPick = jest.fn();
    await render(<SearchResults q="a" onPick={onPick} />);
    await user.press(screen.getByRole('button', { name: 'Add Alien to your watchlist' }));
    expect(mockSetWatchlist).toHaveBeenLastCalledWith({ kind: 'movie', id: '1', title: 'Alien' }, true);
    await user.press(screen.getByRole('button', { name: 'Remove Severance from your watchlist' }));
    expect(mockSetWatchlist).toHaveBeenLastCalledWith({ kind: 'tv', id: '2', title: 'Severance' }, false);
    expect(onPick).not.toHaveBeenCalled();
  });

  it('shows a queued change before it reaches the server', async () => {
    mockQueued = [{ kind: 'watchlist', status: 'pending', meta: { kind: 'movie', id: '3', on: true } }];
    await render(<SearchResults q="a" onPick={jest.fn()} />);
    expect(screen.getByRole('button', { name: 'Remove Heat from your watchlist' })).toBeSelected();
  });

  it('shows the real title for a pasted TMDB link, not the placeholder', async () => {
    const user = userEvent.setup();
    const onPick = jest.fn();
    await render(<SearchResults q="https://www.themoviedb.org/movie/1518204" onPick={onPick} />);
    expect(screen.getByText('Allen')).toBeOnTheScreen();
    expect(screen.queryByText('TMDB movie 1518204')).not.toBeOnTheScreen();
    await user.press(screen.getByRole('button', { name: 'Add Allen to your watchlist' }));
    expect(mockSetWatchlist).toHaveBeenLastCalledWith({ kind: 'movie', id: '1518204', title: 'Allen' }, true);
  });

  it('keeps the placeholder when the linked title cannot be looked up', async () => {
    await render(<SearchResults q="https://www.themoviedb.org/movie/42" onPick={jest.fn()} />);
    expect(screen.getByText('TMDB movie 42')).toBeOnTheScreen();
  });
});
