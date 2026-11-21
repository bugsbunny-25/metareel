import type { AppStateStatus } from 'react-native';

const mockStore = { getItemAsync: jest.fn(), setItemAsync: jest.fn(), deleteItemAsync: jest.fn() };
jest.mock('expo-secure-store', () => ({ ...mockStore, AFTER_FIRST_UNLOCK_THIS_DEVICE_ONLY: 'afterFirstUnlockThisDeviceOnly' }));
jest.mock('expo-crypto', () => ({ getRandomBytes: () => new Uint8Array(16) }));
jest.mock('expo-device', () => ({ modelName: 'Test' }));
jest.mock('@/net/local-network', () => ({ ensureLocalNetworkAccess: jest.fn() }));
jest.mock('@/query/client', () => ({ clearQueryCache: jest.fn() }));

const saved = JSON.stringify({
  server: 'https://metareel.test',
  home: null,
  token: 't',
  sessionId: 1,
  probeKey: 'k',
  expiresAt: new Date(Date.now() + 86_400_000).toISOString(),
  user: { id: 1, username: 'tester', role: 'user' },
});

let appState: AppStateStatus = 'active';
let onChange: ((s: AppStateStatus) => void) | null = null;

beforeEach(() => {
  jest.resetModules();
  jest.clearAllMocks();
  mockStore.setItemAsync.mockResolvedValue(undefined);
  onChange = null;
});

// A fresh session module (its state starts at loading) with AppState faked.
function load() {
  const { AppState } = jest.requireActual<typeof import('react-native')>('react-native');
  Object.defineProperty(AppState, 'currentState', { get: () => appState, configurable: true });
  jest.spyOn(AppState, 'addEventListener').mockImplementation((_t, l) => {
    onChange = l as (s: AppStateStatus) => void;
    return { remove: () => (onChange = null) };
  });
  return jest.requireActual<typeof import('@/auth/session')>('@/auth/session');
}

test('a locked Keychain in the background is not a sign-out: it reads again in front', async () => {
  appState = 'background';
  mockStore.getItemAsync.mockRejectedValueOnce(new Error('errSecInteractionNotAllowed')).mockResolvedValueOnce(saved);
  const session = load();
  await session.restoreSession();
  expect(session.currentSession()).toBeNull();
  expect(onChange).not.toBeNull();

  appState = 'active';
  onChange!('active');
  await session.restoreSession();
  expect(session.currentSession()?.user.username).toBe('tester');
  // Saved again with access after the first unlock, for older sessions.
  expect(mockStore.setItemAsync).toHaveBeenCalledWith('metareel.session', saved, { keychainAccessible: 'afterFirstUnlockThisDeviceOnly' });
});

test('no saved session signs out', async () => {
  appState = 'active';
  mockStore.getItemAsync.mockResolvedValueOnce(null);
  const session = load();
  const seen: string[] = [];
  session.subscribeSession(() => seen.push('change'));
  await session.restoreSession();
  expect(session.currentSession()).toBeNull();
  expect(seen).toEqual(['change']);
});

test('an unreadable Keychain with the app in front signs out instead of loading forever', async () => {
  appState = 'active';
  mockStore.getItemAsync.mockRejectedValueOnce(new Error('broken'));
  const session = load();
  const seen: string[] = [];
  session.subscribeSession(() => seen.push('change'));
  await session.restoreSession();
  expect(seen).toEqual(['change']);
  expect(onChange).toBeNull();
});
