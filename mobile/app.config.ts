import type { ConfigContext, ExpoConfig } from 'expo/config';

import pkg from './package.json';
import { versionCode } from './version.ts';

// The repo and every build are public: nothing secret and no server URL goes
// in here. The server is entered at sign-in. EAS_PROJECT_ID comes from
// `eas init` (see README); without it, OTA updates are simply off.
const projectId = process.env.EAS_PROJECT_ID;
// Android push (watchlist arrivals) goes through FCM, which needs the
// Firebase project's google-services.json; builds without it still work,
// they just can't register for push. Not committed: pass its path.
const googleServicesFile = process.env.GOOGLE_SERVICES_JSON;
// Hosts the app may reach over plain HTTP on the home network (comma
// separated, e.g. "homelab.local,192.168.1.20"). Build-time only; everything
// else must be HTTPS. See plugins/with-home-http.js.
// One version for everything (version.ts): package.json carries it, set by
// `make version V=…` before a release, and the build numbers (iOS buildNumber,
// Android versionCode) come from it, so every later version is higher
// (eas.json reads them from here: appVersionSource local).
const version = pkg.version;
const build = versionCode(version);
const homeHttpHosts = (process.env.HOME_HTTP_HOSTS ?? '')
  .split(',')
  .map((h: string) => h.trim())
  .filter(Boolean);

export default ({ config }: ConfigContext): ExpoConfig => ({
  ...config,
  name: 'metareel',
  slug: 'metareel',
  // The EAS project (expo.dev/accounts/bugsbunny-team/projects/metareel) is the
  // team's, not the signed-in user's; its ID comes from EAS_PROJECT_ID.
  owner: 'bugsbunny-team',
  version,
  scheme: 'metareel',
  platforms: ['ios', 'android'],
  orientation: 'default',
  userInterfaceStyle: 'automatic',
  icon: './assets/images/icon.png',
  backgroundColor: '#0b0b0c',
  ios: {
    bundleIdentifier: 'io.github.bugsbunny25.metareel',
    buildNumber: String(build),
    supportsTablet: true,
    icon: './assets/expo.icon',
    // Only standard HTTPS (exempt), so App Store Connect doesn't ask about
    // export compliance for every TestFlight build.
    infoPlist: { ITSAppUsesNonExemptEncryption: false },
  },
  android: {
    package: 'io.github.bugsbunny25.metareel',
    versionCode: build,
    adaptiveIcon: {
      backgroundColor: '#0b0b0c',
      foregroundImage: './assets/images/android-icon-foreground.png',
      backgroundImage: './assets/images/android-icon-background.png',
      monochromeImage: './assets/images/android-icon-monochrome.png',
    },
    predictiveBackGestureEnabled: true,
    ...(googleServicesFile ? { googleServicesFile } : {}),
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
    // Runs after the extension plugins' project changes (mods run in
    // reverse order): see the file.
    './plugins/with-unique-extension-plists.js',
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
    [
      'expo-sharing',
      {
        // Receive links (and text with a link) shared from other apps.
        ios: { enabled: true, activationRule: { supportsWebUrlWithMaxCount: 1, supportsText: true } },
        android: { enabled: true, singleShareMimeTypes: ['text/plain'] },
      },
    ],
    [
      'expo-notifications',
      {
        // Episode reminders are local (phase 6a); watchlist arrivals are
        // pushed by the server through Expo (phase 6b).
        icon: './assets/images/android-icon-monochrome.png',
        enableRemoteNotifications: true,
        color: '#0b0b0c',
        defaultChannel: 'episodes',
      },
    ],
    [
      'expo-widgets',
      {
        // Home screen widgets from GET /me/glance (phase 7), on both
        // platforms; the iOS lock screen gets Up next as an accessory. The
        // "Now showing" Live Activity needs no entry here.
        enableAndroid: true,
        // Needed for Android custom notifications ("Now showing"); on iOS it
        // adds the push entitlement expo-notifications uses anyway.
        enablePushNotifications: true,
        widgets: [
          {
            name: 'UpNext',
            displayName: 'Up next',
            description: 'The next episode of the shows you are watching, and what airs next.',
            ios: { supportedFamilies: ['systemSmall', 'systemMedium', 'accessoryRectangular', 'accessoryInline'] },
            android: { targetCellWidth: 2, targetCellHeight: 2 },
          },
          {
            name: 'BreakEven',
            displayName: 'Break-even',
            description: 'Your cinema plan this month against the films it takes to pay off.',
            ios: { supportedFamilies: ['systemSmall'] },
            android: { targetCellWidth: 2, targetCellHeight: 2 },
          },
        ],
      },
    ],
    ['./plugins/with-home-http.js', { hosts: homeHttpHosts }],
  ],
  runtimeVersion: { policy: 'fingerprint' },
  ...(projectId
    ? { updates: { url: `https://u.expo.dev/${projectId}` }, extra: { eas: { projectId } } }
    : {}),
  experiments: { typedRoutes: true, reactCompiler: true },
});
