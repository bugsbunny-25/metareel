// Shares from other apps (Netflix, IMDb, Letterboxd…) arrive through
// expo-sharing as metareel://expo-sharing; send them to the share screen,
// which reads the payload. Every other link opens as it is.
export function redirectSystemPath({ path }: { path: string; initial: boolean }) {
  try {
    if (new URL(path).hostname === 'expo-sharing') return '/share';
    return path;
  } catch {
    return path;
  }
}
