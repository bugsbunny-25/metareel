import type { ConfigContext, ExpoConfig } from 'expo/config';

// The repo and every build are public: nothing secret and no server URL goes
// in here. The server is entered at sign-in. EAS_PROJECT_ID comes from
// `eas init` (see README); without it, OTA updates are simply off.
const projectId = process.env.EAS_PROJECT_ID;
// Hosts the app may reach over plain HTTP on the home network (comma
// separated, e.g. "homelab.local,192.168.1.20"). Build-time only; everything
// else must be HTTPS. See plugins/with-home-http.js.
const homeHttpHosts = (process.env.HOME_HTTP_HOSTS ?? '')
  .split(',')
  .map((h: string) => h.trim())
  .filter(Boolean);

export default ({ config }: ConfigContext): ExpoConfig => ({
  ...config,
  name: 'metareel',
  slug: 'metareel',
  version: '0.1.0',
  scheme: 'metareel',
  platforms: ['ios', 'android'],
  orientation: 'default',
  userInterfaceStyle: 'automatic',
  icon: './assets/images/icon.png',
  backgroundColor: '#0b0b0c',
  ios: {
    bundleIdentifier: 'io.github.bugsbunny25.metareel',
    supportsTablet: true,
    icon: './assets/expo.icon',
  },
  android: {
    package: 'io.github.bugsbunny25.metareel',
    adaptiveIcon: {
      backgroundColor: '#0b0b0c',
      foregroundImage: './assets/images/android-icon-foreground.png',
      backgroundImage: './assets/images/android-icon-background.png',
      monochromeImage: './assets/images/android-icon-monochrome.png',
    },
    predictiveBackGestureEnabled: true,
    // Only what the modules declare (no default storage or overlay permissions).
    permissions: [],
    // expo-secure-store adds these for biometric unlock, which the app doesn't use.
    // The rest come from the native template and aren't used.
    blockedPermissions: [
      'android.permission.USE_BIOMETRIC',
      'android.permission.USE_FINGERPRINT',
      'android.permission.SYSTEM_ALERT_WINDOW',
      'android.permission.READ_EXTERNAL_STORAGE',
      'android.permission.WRITE_EXTERNAL_STORAGE',
    ],
  },
  plugins: [
    'expo-router',
    ['expo-splash-screen', { backgroundColor: '#0b0b0c', image: './assets/images/splash-icon.png', imageWidth: 76 }],
    ['expo-secure-store', { faceIDPermission: false }],
    'expo-sqlite',
    [
      'expo-location',
      {
        // Foreground only: the position is used once, to list nearby cinemas.
        locationWhenInUsePermission: 'metareel uses your location when you log a film, to list the cinemas near you.',
        locationAlwaysAndWhenInUsePermission: false,
        locationAlwaysPermission: false,
        motionUsagePermission: false,
        isIosBackgroundLocationEnabled: false,
        isAndroidBackgroundLocationEnabled: false,
        isAndroidForegroundServiceEnabled: false,
      },
    ],
    'expo-background-task',
    ['./plugins/with-home-http.js', { hosts: homeHttpHosts }],
  ],
  runtimeVersion: { policy: 'fingerprint' },
  ...(projectId
    ? { updates: { url: `https://u.expo.dev/${projectId}` }, extra: { eas: { projectId } } }
    : {}),
  experiments: { typedRoutes: true, reactCompiler: true },
});
