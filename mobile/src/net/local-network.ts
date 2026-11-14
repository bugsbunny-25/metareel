import { PermissionsAndroid, Platform } from 'react-native';

// Android 17 (API 37) guards connections to the local network behind a
// runtime permission. Without it the home address can't be reached and the
// app simply keeps using the public server.
const ACCESS_LOCAL_NETWORK = 'android.permission.ACCESS_LOCAL_NETWORK';

export async function ensureLocalNetworkAccess(): Promise<boolean> {
  if (Platform.OS !== 'android' || Number(Platform.Version) < 37) return true;
  const perm = ACCESS_LOCAL_NETWORK as Parameters<typeof PermissionsAndroid.request>[0];
  if (await PermissionsAndroid.check(perm)) return true;
  const res = await PermissionsAndroid.request(perm, {
    title: 'Use your home network?',
    message: 'metareel can talk to your server directly while you are on your home network.',
    buttonPositive: 'Allow',
    buttonNegative: 'Not now',
  });
  return res === PermissionsAndroid.RESULTS.GRANTED;
}
