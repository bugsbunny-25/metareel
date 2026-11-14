import Constants from 'expo-constants';
import { router } from 'expo-router';
import { useState } from 'react';
import { Linking, Platform, Pressable, StyleSheet, Switch, View } from 'react-native';

import { endpoint, setHomeURL, signOut, useSignedIn } from '@/auth/session';
import { normalizeHomeURL, URLError } from '@/auth/urls';
import { Button } from '@/components/Button';
import { Field } from '@/components/Field';
import { DetailScreen, Panel } from '@/components/Screen';
import { Text } from '@/components/Text';
import { useAccount, usePreferences } from '@/data/queries';
import { useAppUpdate } from '@/data/app-update';
import { type ArrivalsResult, setWatchlistArrivals } from '@/notifications/push';
import { setEpisodeReminders, useEpisodeReminders, useReminderStatus } from '@/notifications/reminders';
import { useOutbox } from '@/offline';
import { useEndpointState } from '@/net/endpoint';
import { useTheme } from '@/theme';
import { setThemePreference, type ThemePreference, useThemePreference } from '@/theme/preference';

export default function Settings() {
  const session = useSignedIn();
  const ep = useEndpointState(endpoint);
  const account = useAccount();
  const { colors } = useTheme();
  const [home, setHome] = useState(session.home ?? '');
  const [homeError, setHomeError] = useState<string | null>(null);
  const [signingOut, setSigningOut] = useState(false);
  const queued = useOutbox().length;
  const update = useAppUpdate();

  async function saveHome() {
    setHomeError(null);
    try {
      await setHomeURL(normalizeHomeURL(home, session.server));
    } catch (e) {
      setHomeError(e instanceof URLError ? e.message : 'Check the address.');
    }
  }

  const homeStatus = !session.home
    ? 'Not set: everything goes to the server.'
    : ep.route === 'home'
      ? 'In use: this is your server, checked just now.'
      : ep.checkedAt
        ? 'Not reachable or not your server: using the server address.'
        : 'Checking…';

  return (
    <DetailScreen title="Settings">
      <Panel title="Account">
        <Row label="Signed in as" value={session.user.username} />
        <Row label="Server" value={session.server} />
        <Row label="Session ends" value={new Date(session.expiresAt).toLocaleDateString()} />
      </Panel>

      <Panel title="Home network">
        <Field
          label="Home address (optional)"
          placeholder="http://homelab.local:5080"
          autoCapitalize="none"
          autoCorrect={false}
          keyboardType="url"
          value={home}
          onChangeText={setHome}
          onSubmitEditing={saveHome}
        />
        <Text variant="muted">{homeStatus}</Text>
        {homeError ? <Text style={{ color: colors.deltaDown }}>{homeError}</Text> : null}
        <Button kind="quiet" title="Save" onPress={saveHome} />
      </Panel>

      <Panel title="Cinema subscriptions">
        <Button kind="quiet" title="Break-even and savings" onPress={() => router.push('/subscriptions')} />
      </Panel>

      <Panel title="Appearance">
        <ThemePicker />
      </Panel>

      <Panel title="Notifications">
        <EpisodeReminders />
        <WatchlistArrivals />
      </Panel>

      <Panel title="Where you're signed in">
        {account.data?.sessions?.map((s) => (
          <Row
            key={s.id}
            label={s.device_name || s.user_agent || 'Browser'}
            value={s.current ? 'This device' : `Last used ${new Date(s.last_seen_at ?? '').toLocaleDateString()}`}
          />
        )) ?? <Text variant="muted">{account.isError ? "Couldn't load sessions." : 'Loading…'}</Text>}
      </Panel>

      <Panel title="App">
        <Row label="Version" value={`${Constants.expoConfig?.version ?? '?'}${Platform.OS === 'android' ? ' (APK)' : ''}`} />
        {update.data ? (
          <>
            <Text style={{ color: colors.deltaDebut }}>Version {update.data.version} is available.</Text>
            <Button kind="quiet" title="Download the APK" onPress={() => void Linking.openURL(update.data!.url)} />
          </>
        ) : null}
      </Panel>

      {queued ? (
        <Text variant="muted">
          {queued} change{queued === 1 ? '' : 's'} not yet synced stay on this phone after signing out and go out when this account signs in again.
        </Text>
      ) : null}
      <Button
        kind="quiet"
        title="Sign out"
        busy={signingOut}
        onPress={async () => {
          setSigningOut(true);
          await signOut();
        }}
      />
    </DetailScreen>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <View style={styles.row}>
      <Text variant="muted" style={{ flexShrink: 0 }}>
        {label}
      </Text>
      <Text style={{ flex: 1, textAlign: 'right' }} numberOfLines={1}>
        {value}
      </Text>
    </View>
  );
}

function EpisodeReminders() {
  const on = useEpisodeReminders();
  const scheduled = useReminderStatus();
  const { colors } = useTheme();
  const [busy, setBusy] = useState(false);
  const [blocked, setBlocked] = useState(false);
  return (
    <>
      <View style={styles.row}>
        <View style={{ flex: 1, gap: 2 }}>
          <Text>New episodes</Text>
          <Text variant="muted">At 6 pm on the day a new episode of a show you follow airs. Scheduled on this phone.</Text>
        </View>
        <Switch
          value={on}
          disabled={busy}
          accessibilityLabel="New episode reminders"
          trackColor={{ true: colors.accentYours }}
          thumbColor={Platform.OS === 'android' ? '#ffffff' : undefined}
          onValueChange={async (v) => {
            setBusy(true);
            setBlocked((await setEpisodeReminders(v)) === 'blocked');
            setBusy(false);
          }}
        />
      </View>
      {on ? (
        <Text variant="muted">
          {scheduled.next
            ? `Next: ${scheduled.next.title}, ${scheduled.next.at.toLocaleDateString(undefined, { weekday: 'short', day: 'numeric', month: 'short' })}${scheduled.count > 1 ? ` · ${scheduled.count} scheduled` : ''}`
            : scheduled.count
              ? `${scheduled.count} scheduled`
              : 'Nothing airing in the next two weeks.'}
        </Text>
      ) : null}
      {blocked ? (
        <>
          <Text variant="muted">Notifications are off for metareel. Allow them in the system settings, then turn this on.</Text>
          <Button kind="quiet" title="Open settings" onPress={() => void Linking.openSettings()} />
        </>
      ) : null}
    </>
  );
}

const ARRIVAL_NOTES: Partial<Record<ArrivalsResult, string>> = {
  denied: 'metareel needs permission to send notifications.',
  unavailable: "This build can't receive push notifications (it needs the EAS project and push setup).",
  'server-off': 'Saved, but your server has push notifications off (PUSH_ENABLED).',
};

function WatchlistArrivals() {
  const prefs = usePreferences();
  const on = prefs.data?.notify_watchlist_arrivals === true;
  const country = prefs.data?.country;
  const { colors } = useTheme();
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<ArrivalsResult | null>(null);
  return (
    <>
      <View style={styles.row}>
        <View style={{ flex: 1, gap: 2 }}>
          <Text>Watchlist arrivals</Text>
          <Text variant="muted">
            When a title on your watchlist starts streaming on one of your services{country ? ` in ${country}` : ''}. Sent by your server, once per title.
          </Text>
        </View>
        <Switch
          value={on}
          disabled={busy || !prefs.data}
          accessibilityLabel="Watchlist arrival notifications"
          trackColor={{ true: colors.accentYours }}
          thumbColor={Platform.OS === 'android' ? '#ffffff' : undefined}
          onValueChange={async (v) => {
            setBusy(true);
            try {
              setResult(await setWatchlistArrivals(v));
            } catch {
              setResult(null);
            }
            setBusy(false);
          }}
        />
      </View>
      {on && !country ? <Text variant="muted">Set your country on the web app&apos;s Settings so the server knows where you watch.</Text> : null}
      {result && ARRIVAL_NOTES[result] ? <Text variant="muted">{ARRIVAL_NOTES[result]}</Text> : null}
      {result === 'blocked' ? (
        <>
          <Text variant="muted">Notifications are off for metareel. Allow them in the system settings, then turn this on.</Text>
          <Button kind="quiet" title="Open settings" onPress={() => void Linking.openSettings()} />
        </>
      ) : null}
    </>
  );
}

function ThemePicker() {
  const pref = useThemePreference();
  const { colors } = useTheme();
  const options: { value: ThemePreference; label: string }[] = [
    { value: 'system', label: 'System' },
    { value: 'dark', label: 'Dark' },
    { value: 'light', label: 'Light' },
  ];
  return (
    <View style={[styles.segment, { backgroundColor: colors.bgElevated }]} accessibilityRole="radiogroup">
      {options.map((o) => {
        const on = pref === o.value;
        return (
          <Pressable
            key={o.value}
            accessibilityRole="radio"
            accessibilityState={{ checked: on }}
            onPress={() => setThemePreference(o.value)}
            style={[styles.segmentItem, on && { backgroundColor: colors.accentPrimary }]}
          >
            <Text variant="label" style={{ color: on ? colors.accentOn : colors.textPrimary }}>
              {o.label}
            </Text>
          </Pressable>
        );
      })}
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: 'row', gap: 12, alignItems: 'center', minHeight: 28 },
  segment: { flexDirection: 'row', borderRadius: 20, padding: 3 },
  segmentItem: { flex: 1, minHeight: 36, borderRadius: 17, alignItems: 'center', justifyContent: 'center' },
});
