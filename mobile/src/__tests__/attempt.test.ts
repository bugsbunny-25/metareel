import { attempt, errorMessage } from '@/lib/attempt';
import { chunk } from '@/lib/chunk';

describe('attempt', () => {
  it('resolves to null when the work succeeds, sync or async', async () => {
    const work = jest.fn();
    expect(await attempt(work)).toBeNull();
    expect(work).toHaveBeenCalledTimes(1);
    expect(await attempt(() => Promise.resolve(1))).toBeNull();
  });

  it('resolves to the error message when it throws or rejects', async () => {
    expect(
      await attempt(() => {
        throw new Error('price is not a number');
      }),
    ).toBe('price is not a number');
    expect(await attempt(() => Promise.reject(new Error('offline')))).toBe('offline');
  });

  it('uses the fallback for something that is not an Error', async () => {
    expect(await attempt(() => Promise.reject('nope'), 'Check the form.')).toBe('Check the form.');
    expect(errorMessage(42)).toBe('42');
  });
});

describe('chunk', () => {
  it('splits into rows of n with a shorter last row', () => {
    expect(chunk([1, 2, 3, 4, 5], 2)).toEqual([[1, 2], [3, 4], [5]]);
    expect(chunk([], 3)).toEqual([]);
  });
});
