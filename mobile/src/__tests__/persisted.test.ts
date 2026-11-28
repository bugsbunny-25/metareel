import { act, renderHook } from '@testing-library/react-native';

import { usePersisted } from '@/lib/persisted';

const mockStore = new Map<string, string>();
jest.mock('expo-sqlite/kv-store', () => ({
  __esModule: true,
  default: {
    getItemSync: (k: string) => mockStore.get(k) ?? null,
    setItemSync: (k: string, v: string) => void mockStore.set(k, v),
  },
}));

const VIEWS = ['list', 'grid'] as const;

beforeEach(() => mockStore.clear());

describe('usePersisted', () => {
  it('starts from the initial value and remembers a change', async () => {
    const { result } = await renderHook(() => usePersisted('tabs.tv.view.towatch', 'list', VIEWS));
    expect(result.current[0]).toBe('list');
    await act(() => result.current[1]('grid'));
    expect(result.current[0]).toBe('grid');
    expect(mockStore.get('tabs.tv.view.towatch')).toBe('grid');
  });

  it('keeps a separate choice per key, each starting from the old shared one', async () => {
    mockStore.set('tabs.tv.view', 'grid'); // what an older version saved for both segments
    const towatch = await renderHook(() => usePersisted('tabs.tv.view.towatch', 'list', VIEWS, 'tabs.tv.view'));
    const upcoming = await renderHook(() => usePersisted('tabs.tv.view.upcoming', 'list', VIEWS, 'tabs.tv.view'));
    expect(towatch.result.current[0]).toBe('grid');
    expect(upcoming.result.current[0]).toBe('grid');

    await act(() => upcoming.result.current[1]('list'));
    expect(upcoming.result.current[0]).toBe('list');
    expect(towatch.result.current[0]).toBe('grid');
    expect(mockStore.get('tabs.tv.view')).toBe('grid'); // the old key is left as it was

    // Its own key wins over the old one from then on.
    const again = await renderHook(() => usePersisted('tabs.tv.view.upcoming', 'list', VIEWS, 'tabs.tv.view'));
    expect(again.result.current[0]).toBe('list');
  });

  it('ignores a stored value that is no longer allowed', async () => {
    mockStore.set('tabs.tv.view.towatch', 'carousel');
    const { result } = await renderHook(() => usePersisted('tabs.tv.view.towatch', 'list', VIEWS));
    expect(result.current[0]).toBe('list');
  });
});
