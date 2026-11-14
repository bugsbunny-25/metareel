import { useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { KeyboardAvoidingView, Platform, ScrollView, StyleSheet, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { signIn, SignInError, useSession } from '@/auth/session';
import { normalizeHomeURL, normalizeServerURL, URLError } from '@/auth/urls';
import { Button } from '@/components/Button';
import { Field } from '@/components/Field';
import { Text } from '@/components/Text';
import { useTheme } from '@/theme';

type Prefill = { server?: string; username?: string; password?: string };

export default function SignIn() {
  // Development builds only: metareel://sign-in?server=&username=&password=
  // prefills the form, so simulators can be signed in from a test script
  // (no typing a password through UI automation). Ignored in release builds.
  const dev = useLocalSearchParams<Prefill>();
  const prefill: Prefill = __DEV__ ? { server: dev.server, username: dev.username, password: dev.password } : {};
  return <SignInForm key={__DEV__ ? `${dev.server}|${dev.username}|${dev.password}` : 'form'} prefill={prefill} />;
}

function SignInForm({ prefill }: { prefill: Prefill }) {
  const { colors, gutter } = useTheme();
  const insets = useSafeAreaInsets();
  const session = useSession();
  const [server, setServer] = useState(prefill.server ?? '');
  const [home, setHome] = useState('');
  const [showHome, setShowHome] = useState(false);
  const [username, setUsername] = useState(prefill.username ?? '');
  const [password, setPassword] = useState(prefill.password ?? '');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit() {
    setError(null);
    let serverURL: string;
    let homeURL: string | null;
    try {
      // Development builds may sign in to a local test server over HTTP.
      serverURL = normalizeServerURL(server, { allowHttp: __DEV__ });
      homeURL = normalizeHomeURL(home, serverURL);
    } catch (e) {
      setError(e instanceof URLError ? e.message : 'Check the addresses.');
      return;
    }
    if (!username.trim() || !password) {
      setError('Enter your username and password.');
      return;
    }
    setBusy(true);
    try {
      await signIn({ server: serverURL, home: homeURL, username, password });
    } catch (e) {
      setError(e instanceof SignInError ? e.message : 'Signing in failed.');
      setBusy(false);
    }
  }

  const notice = session.status === 'signedOut' ? session.notice : undefined;
  return (
    <KeyboardAvoidingView style={{ flex: 1, backgroundColor: colors.bgCanvas }} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
      <ScrollView
        keyboardShouldPersistTaps="handled"
        contentContainerStyle={[styles.content, { paddingTop: insets.top + 48, paddingBottom: insets.bottom + 24, paddingHorizontal: gutter.compact }]}
      >
        <View style={styles.form}>
          <Text variant="display">metareel</Text>
          <Text variant="muted">Sign in to your metareel server.</Text>
          {notice ? <Text style={{ color: colors.statusStale }}>{notice}</Text> : null}
          <Field
            label="Server"
            placeholder="https://metareel.example.com"
            autoCapitalize="none"
            autoCorrect={false}
            keyboardType="url"
            textContentType="URL"
            value={server}
            onChangeText={setServer}
          />
          {showHome ? (
            <Field
              label="Home network address (optional)"
              hint="Used only on your home network, after the app checks it is the same server."
              placeholder="http://homelab.local:5080"
              autoCapitalize="none"
              autoCorrect={false}
              keyboardType="url"
              value={home}
              onChangeText={setHome}
            />
          ) : (
            <Button kind="quiet" title="Add a home network address" onPress={() => setShowHome(true)} />
          )}
          <Field
            label="Username"
            autoCapitalize="none"
            autoCorrect={false}
            textContentType="username"
            autoComplete="username"
            value={username}
            onChangeText={setUsername}
          />
          <Field
            label="Password"
            secureTextEntry
            textContentType="password"
            autoComplete="current-password"
            value={password}
            onChangeText={setPassword}
            onSubmitEditing={submit}
            returnKeyType="go"
          />
          {error ? (
            <Text accessibilityLiveRegion="polite" style={{ color: colors.deltaDown }}>
              {error}
            </Text>
          ) : null}
          <Button title="Sign in" busy={busy} onPress={submit} />
        </View>
      </ScrollView>
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  content: { flexGrow: 1, alignItems: 'center' },
  form: { width: '100%', maxWidth: 440, gap: 16 },
});
