import { useQuery } from '@tanstack/react-query';
import * as Crypto from 'expo-crypto';
import * as Haptics from 'expo-haptics';
import Storage from 'expo-sqlite/kv-store';
import { router, useLocalSearchParams } from 'expo-router';
import { type ReactNode, useEffect, useState } from 'react';
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, useWindowDimensions, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { unwrap } from '@/api/client';
import type { components } from '@/api/schema';
import { api } from '@/auth/session';
import { Chip } from '@/components/Chip';
import { Field } from '@/components/Field';
import { Icon } from '@/components/Icon';
import { Poster } from '@/components/Poster';
import { Stars } from '@/components/Stars';
import { Text } from '@/components/Text';
import { findNearby, type NearbyResult, type NearbyVenue } from '@/data/nearby';
import { useFormats, useMyTitle, usePreferences, useSubscriptions, useWatch } from '@/data/queries';
import { useWindowClass } from '@/layout/window-class';
import { isTabletop, useFold } from 'fold-info';
import { currencyFor, deviceTimeZone, endedAt, episodeCode, toMinor, type When } from '@/lib/format';
import { enqueue } from '@/offline';
import { queryClient } from '@/query/client';
import { useTheme } from '@/theme';

type WatchInput = components['schemas']['WatchInput'];
type Place = '' | 'theater' | 'streaming' | 'physical' | 'broadcast';
type Venue = Pick<NearbyVenue, 'id' | 'name' | 'city' | 'timezone' | 'chain'> & { distance_m?: number };

type Draft = {
  when: When;
  rating: number | null;
  liked: boolean;
  notes: string;
  place: Place;
  venue: Venue | null;
  formats: string[];
  price: string;
  tickets: string;
  currency: string;
  subscriptionId: number | null;
  faceValue: string;
  provider: string;
  savedAt: number;
};

const WHEN: { value: When; label: string }[] = [
  { value: 'now', label: 'Just now' },
  { value: 'hour', label: 'An hour ago' },
  { value: 'today', label: 'Earlier today' },
  { value: 'yesterday', label: 'Yesterday' },
];
const PLACES: { value: Place; label: string }[] = [
  { value: 'theater', label: 'Cinema' },
  { value: 'streaming', label: 'Streaming' },
  { value: 'physical', label: 'Disc' },
  { value: 'broadcast', label: 'TV' },
];
const DRAFT_TTL = 6 * 60 * 60_000;

/**
 * Log a watch (or edit one: ?watchId=). Quick first: when, stars, liked, then
 * where. A new watch is queued in the outbox with a client_key, so it is kept
 * offline and never logged twice; editing needs the network.
 */
export default function LogWatch() {
  const p = useLocalSearchParams<{ kind: string; id: string; title?: string; poster?: string; season?: string; episode?: string; watchId?: string }>();
  const watchId = p.watchId ? Number(p.watchId) : null;
  return watchId ? <EditWatch watchId={watchId} /> : <NewWatch params={p} />;
}

function NewWatch({ params: p }: { params: { kind: string; id: string; title?: string; poster?: string; season?: string; episode?: string } }) {
  const kind = p.kind === 'tv' ? 'tv' : 'movie';
  const season = p.season ? Number(p.season) : null;
  const episode = p.episode ? Number(p.episode) : null;
  const draftKey = `draft:${kind}:${p.id}:${season ?? ''}:${episode ?? ''}`;
  const prefs = usePreferences();
  const [d, setD] = useState<Draft>(() => loadDraft(draftKey) ?? emptyDraft(''));
  const set = (patch: Partial<Draft>) => setD((x) => ({ ...x, ...patch }));
  const [error, setError] = useState<string | null>(null);

  // The currency follows your preference until you type one.
  const currency = d.currency || prefs.data?.currency || currencyFor(prefs.data?.country);
  // Keep the half-filled form across app switches and restarts.
  useEffect(() => {
    try {
      Storage.setItemSync(draftKey, JSON.stringify({ ...d, savedAt: Date.now() }));
    } catch {
      // A draft that can't be saved only lives while the sheet is open.
    }
  }, [d, draftKey]);

  function submit() {
    setError(null);
    let body: WatchInput;
    try {
      body = buildInput(kind, p.id, season, episode, { ...d, currency });
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Check the form.');
      return;
    }
    const label = `${p.title ?? 'Title'}${season != null ? ` ${episodeCode(season, episode)}` : ''}`;
    enqueue({
      kind: 'watch',
      method: 'POST',
      path: '/api/v1/me/watches',
      body,
      label,
      meta: { title: p.title, poster: p.poster, ended_at: body.ended_at, rating: body.rating, tmdb_kind: kind, tmdb_id: p.id },
    });
    try {
      Storage.removeItemSync(draftKey);
    } catch {
      // Left behind, it expires after a few hours.
    }
    void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
    router.back();
  }

  return (
    <Sheet
      title={p.title ?? ''}
      subtitle={season != null ? episodeCode(season, episode) : undefined}
      poster={p.poster}
      action={<SheetAction title="Log" onPress={submit} />}
      error={error}
    >
      <Section title="When">
        <Row>
          {WHEN.map((w) => (
            <Chip key={w.value} label={w.label} selected={d.when === w.value} onPress={() => set({ when: w.value })} />
          ))}
        </Row>
      </Section>
      <Section title="Rating">
        <View style={styles.ratingRow}>
          <Stars value={d.rating} onChange={(rating) => set({ rating })} />
          <LikeToggle liked={d.liked} onChange={(liked) => set({ liked })} />
        </View>
      </Section>
      <Section title="Where">
        <Row>
          {PLACES.map((pl) => (
            <Chip key={pl.value} label={pl.label} selected={d.place === pl.value} onPress={() => set({ place: d.place === pl.value ? '' : pl.value })} />
          ))}
        </Row>
      </Section>
      {d.place === 'theater' ? <CinemaFields d={{ ...d, currency }} set={set} /> : null}
      {d.place === 'streaming' ? (
        <Field label="Service" placeholder="Netflix" value={d.provider} onChangeText={(provider) => set({ provider })} autoCorrect={false} />
      ) : null}
      <Field label="Notes" multiline value={d.notes} onChangeText={(notes) => set({ notes })} style={{ minHeight: 80, paddingTop: 12, textAlignVertical: 'top' }} />
    </Sheet>
  );
}

function CinemaFields({ d, set }: { d: Draft; set: (p: Partial<Draft>) => void }) {
  const { colors } = useTheme();
  const formats = useFormats();
  const subs = useSubscriptions();
  const [nearby, setNearby] = useState<NearbyResult | null>(null);
  const [attempt, setAttempt] = useState(0);
  const [query, setQuery] = useState('');
  const search = useQuery({
    queryKey: ['venue-search', query.trim()],
    enabled: query.trim().length >= 2,
    queryFn: async ({ signal }) =>
      unwrap(await api.GET('/api/v1/me/venues', { params: { query: { q: query.trim(), include_closed: true, limit: 8 } }, signal })),
  });

  useEffect(() => {
    if (d.venue) return;
    let live = true;
    void findNearby().then((r) => live && setNearby(r));
    return () => {
      live = false;
    };
  }, [d.venue, attempt]);

  const activeSubs = (subs.data?.subscriptions ?? []).filter((s) => !s.ended_on);
  const cinemaFormats = (formats.data?.formats ?? []).filter((f) => f.active !== false && ['brand', 'projection', 'sound', 'dimension'].includes(f.category ?? ''));

  return (
    <>
      <Section title="Cinema">
        {d.venue ? (
          <Pressable onPress={() => set({ venue: null })} style={[styles.venuePicked, { backgroundColor: colors.bgElevated }]} accessibilityRole="button" accessibilityHint="Choose a different cinema">
            <Icon name="pin" color={colors.textPrimary} />
            <View style={{ flex: 1 }}>
              <Text variant="label">{d.venue.name}</Text>
              {d.venue.city ? <Text variant="muted">{d.venue.city}</Text> : null}
            </View>
            <Text variant="caption">Change</Text>
          </Pressable>
        ) : (
          <>
            {!nearby ? <ActivityIndicator color={colors.textMuted} /> : null}
            {nearby?.status === 'denied' ? <Text variant="muted">Location is off for metareel. Search for the cinema instead.</Text> : null}
            {nearby?.status === 'unavailable' ? (
              <Pressable onPress={() => { setNearby(null); setAttempt((a) => a + 1); }} accessibilityRole="button">
                <Text variant="muted">
                  {nearby.message} <Text variant="label">Try again</Text>
                </Text>
              </Pressable>
            ) : null}
            {nearby?.status === 'ok' && nearby.offline ? <Text variant="caption">Offline: from saved cinemas near here</Text> : null}
            {nearby?.status === 'ok'
              ? nearby.venues.map((v) => <VenueRow key={v.id} v={v} onPress={() => set({ venue: v })} />)
              : null}
            <Field label="Search cinemas" placeholder="PVR Phoenix" value={query} onChangeText={setQuery} autoCorrect={false} />
            {(search.data?.venues ?? []).map((v) => (
              <VenueRow key={v.id} v={{ ...v, chain: v.chain ?? undefined, city: v.city ?? undefined, timezone: v.timezone ?? undefined } as Venue} onPress={() => set({ venue: v as Venue })} />
            ))}
          </>
        )}
      </Section>
      {cinemaFormats.length ? (
        <Section title="Format">
          <Row>
            {cinemaFormats.map((f) => (
              <Chip
                key={f.slug}
                label={f.label ?? f.slug ?? ''}
                selected={d.formats.includes(f.slug!)}
                onPress={() => set({ formats: d.formats.includes(f.slug!) ? d.formats.filter((s) => s !== f.slug) : [...d.formats, f.slug!] })}
              />
            ))}
          </Row>
        </Section>
      ) : null}
      {activeSubs.length ? (
        <Section title="Covered by a subscription">
          <Row>
            {activeSubs.map((s) => (
              <Chip key={s.id} label={s.plan ?? ''} selected={d.subscriptionId === s.id} onPress={() => set({ subscriptionId: d.subscriptionId === s.id ? null : s.id! })} />
            ))}
          </Row>
          {d.subscriptionId ? (
            <Field label={`Ticket face value (${d.currency})`} keyboardType="decimal-pad" value={d.faceValue} onChangeText={(faceValue) => set({ faceValue })} hint="What the ticket would have cost; leave empty to use your usual price." />
          ) : null}
        </Section>
      ) : null}
      {!d.subscriptionId ? (
        <View style={styles.money}>
          <View style={{ flex: 2 }}>
            <Field label="Paid" keyboardType="decimal-pad" placeholder="0.00" value={d.price} onChangeText={(price) => set({ price })} />
          </View>
          <View style={{ flex: 1 }}>
            <Field label="Currency" autoCapitalize="characters" maxLength={3} value={d.currency} onChangeText={(currency) => set({ currency })} />
          </View>
          <View style={{ flex: 1 }}>
            <Field label="Tickets" keyboardType="number-pad" value={d.tickets} onChangeText={(tickets) => set({ tickets })} />
          </View>
        </View>
      ) : null}
    </>
  );
}

function VenueRow({ v, onPress }: { v: Venue; onPress: () => void }) {
  const { colors } = useTheme();
  const dist = v.distance_m == null ? '' : v.distance_m < 1000 ? `${v.distance_m} m` : `${(v.distance_m / 1000).toFixed(1)} km`;
  return (
    <Pressable onPress={onPress} style={({ pressed }) => [styles.venue, { opacity: pressed ? 0.7 : 1 }]} accessibilityRole="button">
      <View style={{ flex: 1 }}>
        <Text variant="label" numberOfLines={1}>
          {v.name}
          {(v as NearbyVenue).your_watches ? <Text variant="label" style={{ color: colors.accentYours }}> · yours</Text> : null}
        </Text>
        {v.city ? <Text variant="muted" numberOfLines={1}>{v.city}</Text> : null}
      </View>
      {dist ? (
        <Text variant="caption" num>
          {dist}
        </Text>
      ) : null}
    </Pressable>
  );
}

/** Edit a logged watch: rating, liked and notes (the rest stays as logged). */
function EditWatch({ watchId }: { watchId: number }) {
  const { colors } = useTheme();
  const w = useWatch(watchId);
  if (!w.data) {
    return (
      <Sheet title="" action={null} error={w.isError ? "Couldn't load this watch." : null}>
        <ActivityIndicator color={colors.textMuted} />
      </Sheet>
    );
  }
  return <EditForm key={watchId} entry={w.data} />;
}

function EditForm({ entry: e }: { entry: components['schemas']['WatchEntry'] }) {
  // One watch carries no title or poster; the title's own record has them.
  const t = useMyTitle(e.tmdb_kind ?? 'movie', e.tmdb_id ?? '');
  const [rating, setRating] = useState<number | null>(e.rating ?? null);
  const [liked, setLiked] = useState(!!e.liked);
  const [notes, setNotes] = useState(e.notes ?? '');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function save() {
    setBusy(true);
    setError(null);
    try {
      unwrap(
        await api.PUT('/api/v1/me/watches/{id}', {
          params: { path: { id: e.id! } },
          body: { ...inputFromEntry(e), rating, liked, notes: notes.trim() || null },
        }),
      );
      void queryClient.invalidateQueries();
      router.back();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Couldn't save. Are you online?");
      setBusy(false);
    }
  }

  return (
    <Sheet
      title={e.title ?? t.data?.title?.title ?? ''}
      subtitle={e.subject_kind === 'episode' ? episodeCode(e.season_number, e.episode_number) : undefined}
      poster={e.poster_path ?? t.data?.title?.poster_path}
      action={<SheetAction title="Save" busy={busy} onPress={save} />}
      error={error}
    >
      <Section title="Rating">
        <View style={styles.ratingRow}>
          <Stars value={rating} onChange={setRating} />
          <LikeToggle liked={liked} onChange={setLiked} />
        </View>
      </Section>
      <Field label="Notes" multiline value={notes} onChangeText={setNotes} style={{ minHeight: 80, paddingTop: 12, textAlignVertical: 'top' }} />
      <Text variant="muted">Date, place and the rest are edited on the web.</Text>
    </Sheet>
  );
}

// --- building the request

function buildInput(kind: 'movie' | 'tv', id: string, season: number | null, episode: number | null, d: Draft): WatchInput {
  const out: WatchInput = {
    client_key: Crypto.randomUUID(),
    tmdb_kind: kind,
    tmdb_id: id,
    ...endedAt(d.when),
    // A cinema's own zone decides the day of a cinema watch.
    tz: (d.place === 'theater' && d.venue?.timezone) || deviceTimeZone(),
    rating: d.rating,
    liked: d.liked,
    notes: d.notes.trim() || null,
    venue_kind: d.place || null,
    formats: d.place === 'theater' ? d.formats : [],
  };
  if (kind === 'tv') Object.assign(out, { season_number: season, episode_number: episode });
  if (d.place === 'theater') {
    const currency = d.currency.trim().toUpperCase();
    const price = toMinor(d.price, currency || 'USD');
    const face = toMinor(d.faceValue, currency || 'USD');
    if (price === undefined) throw new Error('"Paid" must be an amount, like 12.50.');
    if (face === undefined) throw new Error('The face value must be an amount, like 15.00.');
    const tickets = d.tickets.trim() ? Number(d.tickets) : null;
    if (tickets != null && (!Number.isInteger(tickets) || tickets < 1)) throw new Error('Tickets must be a whole number.');
    const amounts = d.subscriptionId ? { face_value_minor: face } : { price_minor: price };
    if (Object.values(amounts).some((v) => v != null) && !/^[A-Z]{3}$/.test(currency)) {
      throw new Error('Pick a currency for the amount, e.g. USD.');
    }
    Object.assign(out, {
      venue_id: d.venue?.id ?? null,
      subscription_id: d.subscriptionId,
      tickets_count: tickets,
      ...amounts,
      currency: Object.values(amounts).some((v) => v != null) ? currency : null,
    });
  }
  if (d.place === 'streaming') Object.assign(out, { provider_name: d.provider.trim() || null });
  return out;
}

/** A watch as it was logged, as a full input (PUT replaces the watch). */
export function inputFromEntry(e: components['schemas']['WatchEntry']): WatchInput {
  const { id: _id, client_key: _ck, subject_kind: _sk, runtime_minutes_used: _r, runtime_source: _rs, watched_on: _wo, is_rewatch: _ir,
    watch_number: _wn, source: _s, created_at: _c, updated_at: _u, title: _t, release_date: _rd, poster_path: _pp, formats, ...rest } = e;
  return {
    ...(rest as Omit<WatchInput, 'formats'>),
    tmdb_kind: e.tmdb_kind ?? 'movie',
    tmdb_id: e.tmdb_id ?? '',
    ended_at: e.ended_at ?? new Date().toISOString(),
    time_precision: e.time_precision ?? 'minute',
    venue_kind: (e.venue_kind as WatchInput['venue_kind']) ?? null,
    formats: (formats ?? []).map((f) => f.slug!).filter(Boolean),
  };
}

function emptyDraft(currency: string): Draft {
  return {
    when: 'now', rating: null, liked: false, notes: '', place: '', venue: null, formats: [],
    price: '', tickets: '1', currency, subscriptionId: null, faceValue: '', provider: '', savedAt: Date.now(),
  };
}

function loadDraft(key: string): Draft | null {
  try {
    const raw = Storage.getItemSync(key);
    if (!raw) return null;
    const d = JSON.parse(raw) as Draft;
    return Date.now() - d.savedAt < DRAFT_TTL ? d : null;
  } catch {
    return null;
  }
}

// --- layout pieces

/**
 * The sheet: title row with Close and the primary action (always visible,
 * whatever height the sheet is pulled to), then the form.
 */
function Sheet({ title, subtitle, poster, action, error, children }: {
  title: string; subtitle?: string; poster?: string | null; action: ReactNode; error?: string | null; children: ReactNode;
}) {
  const { colors, gutter, radius } = useTheme();
  const insets = useSafeAreaInsets();
  const { width } = useWindowDimensions();
  const sidePanel = useWindowClass() !== 'compact';
  const fold = useFold();
  if (isTabletop(fold)) {
    // Tabletop (a half-open foldable on a table): the title and artwork stand
    // up on the top half, the form lies flat below the fold.
    return (
      <View style={{ flex: 1, backgroundColor: colors.bgSurface }}>
        <View style={[styles.tableTop, { height: fold.top, paddingTop: insets.top + 8, paddingHorizontal: gutter.compact }]}>
          <Poster path={poster} title={title} width={Math.max(60, Math.min(140, Math.round((fold.top - insets.top - 24) / 1.5)))} />
          <View style={{ flex: 1, gap: 8 }}>
            <Text variant="display" numberOfLines={3}>{title}</Text>
            {subtitle ? <Text variant="muted" num>{subtitle}</Text> : null}
            <View style={{ flexDirection: 'row', gap: 8 }}>
              {action}
              <Pressable onPress={() => router.back()} accessibilityRole="button" accessibilityLabel="Close" style={[styles.close, { backgroundColor: colors.bgElevated }]}>
                <Text variant="label">✕</Text>
              </Pressable>
            </View>
          </View>
        </View>
        <View style={{ height: fold.height }} />
        {error ? <Text style={{ color: colors.deltaDown, paddingHorizontal: gutter.compact }}>{error}</Text> : null}
        <ScrollView keyboardShouldPersistTaps="handled" contentContainerStyle={{ padding: gutter.compact, gap: 18, paddingBottom: insets.bottom + 24 }}>
          {children}
        </ScrollView>
      </View>
    );
  }
  const sheet = (
    <View style={{ flex: 1, backgroundColor: colors.bgSurface }}>
      <View style={[styles.head, { paddingHorizontal: gutter.compact }]}>
        <Poster path={poster} title={title} width={44} />
        <View style={{ flex: 1 }}>
          <Text variant="heading" numberOfLines={2}>{title}</Text>
          {subtitle ? <Text variant="muted" num>{subtitle}</Text> : null}
        </View>
        {action}
        <Pressable onPress={() => router.back()} accessibilityRole="button" accessibilityLabel="Close" style={[styles.close, { backgroundColor: colors.bgElevated }]}>
          <Text variant="label">✕</Text>
        </Pressable>
      </View>
      {error ? (
        <Text style={{ color: colors.deltaDown, paddingHorizontal: gutter.compact }} accessibilityLiveRegion="polite">
          {error}
        </Text>
      ) : null}
      <ScrollView keyboardShouldPersistTaps="handled" contentContainerStyle={{ padding: gutter.compact, gap: 18, paddingBottom: insets.bottom + 24 }}>
        {children}
      </ScrollView>
    </View>
  );
  if (!sidePanel) return sheet;
  // Wide windows: the form slides in beside the content, which stays visible
  // (dimmed) behind it; tapping outside closes it.
  return (
    <View style={styles.overlay}>
      <Pressable style={[StyleSheet.absoluteFill, { backgroundColor: 'rgba(0,0,0,0.45)' }]} onPress={() => router.back()} accessibilityLabel="Close" />
      <View
        style={[
          styles.panel,
          {
            width: Math.min(480, Math.round(width * 0.5)),
            paddingTop: insets.top,
            borderTopLeftRadius: radius.hero,
            borderBottomLeftRadius: radius.hero,
            backgroundColor: colors.bgSurface,
          },
        ]}
      >
        {sheet}
      </View>
    </View>
  );
}

function SheetAction({ title, onPress, busy }: { title: string; onPress: () => void; busy?: boolean }) {
  const { colors } = useTheme();
  return (
    <Pressable
      onPress={onPress}
      disabled={busy}
      accessibilityRole="button"
      style={({ pressed }) => [styles.action, { backgroundColor: colors.accentPrimary, opacity: pressed || busy ? 0.8 : 1 }]}
    >
      {busy ? <ActivityIndicator color={colors.accentOn} /> : <Text variant="label" style={{ color: colors.accentOn, fontSize: 16 }}>{title}</Text>}
    </Pressable>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <View style={{ gap: 8 }}>
      <Text variant="caption">{title}</Text>
      {children}
    </View>
  );
}

function Row({ children }: { children: ReactNode }) {
  return <View style={styles.wrapRow}>{children}</View>;
}

function LikeToggle({ liked, onChange }: { liked: boolean; onChange: (v: boolean) => void }) {
  const { colors } = useTheme();
  return (
    <Pressable
      onPress={() => {
        void Haptics.selectionAsync();
        onChange(!liked);
      }}
      accessibilityRole="switch"
      accessibilityState={{ checked: liked }}
      accessibilityLabel="Liked"
      style={[styles.like, { backgroundColor: liked ? colors.accentYours : colors.bgElevated }]}
    >
      <Icon name={liked ? 'heart-fill' : 'heart'} size={22} color={liked ? colors.accentOn : colors.textPrimary} />
    </Pressable>
  );
}

const styles = StyleSheet.create({
  head: { flexDirection: 'row', alignItems: 'center', gap: 12, paddingTop: 20, paddingBottom: 8 },
  overlay: { flex: 1, flexDirection: 'row', justifyContent: 'flex-end' },
  tableTop: { flexDirection: 'row', alignItems: 'flex-end', gap: 16, paddingBottom: 12 },
  panel: { height: '100%', overflow: 'hidden' },
  close: { width: 36, height: 36, borderRadius: 18, alignItems: 'center', justifyContent: 'center' },
  action: { height: 40, minWidth: 72, paddingHorizontal: 18, borderRadius: 20, alignItems: 'center', justifyContent: 'center' },
  wrapRow: { flexDirection: 'row', flexWrap: 'wrap', gap: 8 },
  ratingRow: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' },
  like: { width: 44, height: 44, borderRadius: 22, alignItems: 'center', justifyContent: 'center' },
  venue: { flexDirection: 'row', alignItems: 'center', gap: 12, paddingVertical: 8 },
  venuePicked: { flexDirection: 'row', alignItems: 'center', gap: 12, padding: 12, borderRadius: 12 },
  money: { flexDirection: 'row', gap: 10 },
});
