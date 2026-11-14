import { useSyncExternalStore } from 'react';
import Storage from 'expo-sqlite/kv-store';

/** The theme the user picked on this device. "system" follows the OS. */
export type ThemePreference = 'system' | 'dark' | 'light';

const KEY = 'theme.preference';
const listeners = new Set<() => void>();
let current: ThemePreference = read();

function read(): ThemePreference {
  try {
    const v = Storage.getItemSync(KEY);
    return v === 'dark' || v === 'light' || v === 'system' ? v : 'system';
  } catch {
    return 'system';
  }
}

export function setThemePreference(p: ThemePreference) {
  current = p;
  try {
    Storage.setItemSync(KEY, p);
  } catch {
    // A preference that isn't saved only lasts until the app restarts.
  }
  listeners.forEach((l) => l());
}

export function useThemePreference(): ThemePreference {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    () => current,
  );
}
