import { act, renderHook } from '@testing-library/react-native';

import { usePullToRefresh } from '@/lib/refresh';

describe('usePullToRefresh', () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  it('is idle until pulled, however the queries refetch on their own', async () => {
    const { result } = await renderHook(() => usePullToRefresh(() => Promise.resolve()));
    expect(result.current.refreshing).toBe(false);
  });

  it('ends after every refetch, shown at least briefly', async () => {
    let finish!: () => void;
    const refetch = jest.fn(() => new Promise<void>((r) => (finish = r)));
    const { result } = await renderHook(() => usePullToRefresh(refetch));
    await act(async () => result.current.onRefresh());
    expect(result.current.refreshing).toBe(true);
    expect(refetch).toHaveBeenCalledTimes(1);
    await act(async () => {
      finish();
      await Promise.resolve();
    });
    expect(result.current.refreshing).toBe(true); // the minimum time is not up
    await act(async () => {
      jest.advanceTimersByTime(500);
    });
    expect(result.current.refreshing).toBe(false);
  });

  it('ends when a refetch fails', async () => {
    const { result } = await renderHook(() => usePullToRefresh(() => Promise.reject(new Error('offline'))));
    await act(async () => result.current.onRefresh());
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      jest.advanceTimersByTime(500);
    });
    expect(result.current.refreshing).toBe(false);
  });

  it('ends after 15 seconds when a refetch never settles (paused offline)', async () => {
    const { result } = await renderHook(() => usePullToRefresh(() => new Promise(() => undefined)));
    await act(async () => result.current.onRefresh());
    await act(async () => {
      jest.advanceTimersByTime(14_000);
    });
    expect(result.current.refreshing).toBe(true);
    await act(async () => {
      jest.advanceTimersByTime(1_000);
      await Promise.resolve();
      await Promise.resolve();
      jest.advanceTimersByTime(10);
    });
    expect(result.current.refreshing).toBe(false);
  });
});
