import Storage from 'expo-sqlite/kv-store';
import { useCallback, useState } from 'react';

/** A small UI choice (a tab's segment, list or grid) kept across launches. */
export function usePersisted<T extends string>(key: string, initial: T, allowed: readonly T[]): [T, (v: T) => void] {
  const [value, setValue] = useState<T>(() => {
    try {
      const v = Storage.getItemSync(key) as T | null;
      return v && allowed.includes(v) ? v : initial;
    } catch {
      return initial;
    }
  });
  const set = useCallback(
    (v: T) => {
      setValue(v);
      try {
        Storage.setItemSync(key, v);
      } catch {
        // Remembered for this run only.
      }
    },
    [key],
  );
  return [value, set];
}
