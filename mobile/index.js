// Background tasks must be defined before the app renders, including when
// the OS starts the app headless to run them.
import './src/offline/background';

// Register the app through Expo Router (last, after the side effects above).
import 'expo-router/entry';
