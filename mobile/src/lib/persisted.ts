import Storage from 'expo-sqlite/kv-store';
import { useState } from 'react';

function read<T extends string>(key: string, allowed: readonly T[]): T | null {
  try {
    const v = Storage.getItemSync(key) as T | null;
    return v && allowed.includes(v) ? v : null;
  } catch {
    return null;
  }
}

function write(key: string, v: string) {
  try {
    Storage.setItemSync(key, v);
  } catch {
    // Remembered for this run only.
  }
}

/**
 * A small UI choice (a tab's segment, list or grid) kept across launches.
 * `fallbackKey` is where an older version kept it: its value is the starting
 * point until this key has one of its own.
 */
export function usePersisted<T extends string>(key: string, initial: T, allowed: readonly T[], fallbackKey?: string): [T, (v: T) => void] {
  const [value, setValue] = useState<T>(() => read(key, allowed) ?? (fallbackKey ? read(fallbackKey, allowed) : null) ?? initial);
  const set = (v: T) => {
    setValue(v);
    write(key, v);
  };
  return [value, set];
}
