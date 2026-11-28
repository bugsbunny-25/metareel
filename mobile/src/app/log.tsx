import * as Crypto from 'expo-crypto';
import * as Haptics from 'expo-haptics';
import Storage from 'expo-sqlite/kv-store';
import { router, useLocalSearchParams } from 'expo-router';
import { type ReactNode, useEffect, useEffectEvent, useState } from 'react';
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, useWindowDimensions, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { unwrap } from '@/api/client';
import type { components } from '@/api/schema';
import { api } from '@/auth/session';
import { Button } from '@/components/Button';
import { Field } from '@/components/Field';
import { Poster } from '@/components/Poster';
import { Text } from '@/components/Text';
import { useMyTitle, usePreferences, useSubscriptions, useWatch } from '@/data/queries';
import { useWindowClass } from '@/layout/window-class';
import { isBookSplit, isTabletop, useFold } from 'fold-info';
import { attempt } from '@/lib/attempt';
import { currencyFor, deviceTimeZone, episodeCode } from '@/lib/format';
import { blankForm, buildInput, FORM_VERSION, formDay, formFromEntry, type LogForm, planOnDay, suggestPlan } from '@/log/form';
import { PictureSection, Section, TakeSection, WhenSection, WhereSection } from '@/log/sections';
import { loggedTitle, startNowShowing, useNowShowing } from '@/nowshowing';
import { enqueue } from '@/offline';
import { queryClient } from '@/query/client';
import { useTheme } from '@/theme';

type WatchInput = components['schemas']['WatchInput'];

/**
 * Log a watch (or edit one: ?watchId=). Quick first: when, stars, liked, then
 * where, with every detail the web form has (cinema, plan, money, formats,
 * disc, picture and sound). A new watch is queued in the outbox with a
 * client_key, so it is kept offline and never logged twice; an edit replaces
 * the whole watch and needs the network.
 */
export default function LogWatch() {
  const p = useLocalSearchParams<{ kind: string; id: string; title?: string; poster?: string; season?: string; episode?: string; watchId?: string }>();
  const watchId = p.watchId ? Number(p.watchId) : null;
  return watchId ? <EditWatch watchId={watchId} /> : <NewWatch params={p} />;
}

const DRAFT_TTL = 6 * 60 * 60_000;

function NewWatch({ params: p }: { params: { kind: string; id: string; title?: string; poster?: string; season?: string; episode?: string } }) {
  const kind = p.kind === 'tv' ? 'tv' : 'movie';
  const season = p.season ? Number(p.season) : null;
  const episode = p.episode ? Number(p.episode) : null;
  const draftKey = `draft:${kind}:${p.id}:${season ?? ''}:${episode ?? ''}`;
  const prefs = usePreferences();
  const meta = useMyTitle(kind, p.id, kind === 'movie' && season == null);
  const showing = useNowShowing();
  const [f, setF] = useState<LogForm>(() => loadDraft(draftKey) ?? blankForm({ zone: deviceTimeZone() }));
  const set = (patch: Partial<LogForm>) => setF((x) => ({ ...x, ...patch }));
  const [error, setError] = useState<string | null>(null);
  usePlanSuggestion(f, set);

  // The currency follows your preference until you type one.
  const currency = f.currency || prefs.data?.currency || currencyFor(prefs.data?.country);
  // Keep the half-filled form across app switches and restarts.
  useEffect(() => {
    try {
      Storage.setItemSync(draftKey, JSON.stringify({ ...f, savedAt: Date.now() }));
    } catch {
      // A draft that can't be saved only lives while the sheet is open.
    }
  }, [f, draftKey]);

  function submit() {
    setError(null);
    let body: WatchInput;
    try {
      body = { ...buildInput({ ...f, currency }, { kind, id: p.id, season, episode }), client_key: Crypto.randomUUID() };
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
    loggedTitle(kind, p.id);
    void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
    router.back();
  }

  // "Now showing": watching it right now. The form is kept as a draft, and
  // "Log it" from the lock screen / notification brings it back.
  function startShowing() {
    void startNowShowing({
      kind,
      id: p.id,
      title: p.title ?? meta.data?.title?.title ?? 'Film',
      poster: p.poster ?? '',
      venue: f.place === 'theater' ? (f.venue?.name ?? '') : '',
      runtime: meta.data?.title?.runtime || null,
    });
    void Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
    router.back();
  }
  const canShow = kind === 'movie' && season == null && !(showing?.kind === kind && showing.id === p.id);

  return (
    <Sheet
      title={p.title ?? ''}
      subtitle={season != null ? episodeCode(season, episode) : undefined}
      poster={p.poster}
      action={<SheetAction title="Log" onPress={submit} />}
      error={error}
    >
      <FormBody f={{ ...f, currency }} set={set} isNew country={prefs.data?.country} />
      {canShow ? (
        <Section title="Watching it now?">
          <Text variant="muted">Keep it on your lock screen until it ends, then log it from there with this form filled in.</Text>
          <Button kind="quiet" title="Start Now showing" onPress={startShowing} />
        </Section>
      ) : null}
    </Sheet>
  );
}

/** The form's sections, the same for a new watch and an edit. */
function FormBody({ f, set, isNew, country }: { f: LogForm; set: (p: Partial<LogForm>) => void; isNew: boolean; country?: string }) {
  return (
    <>
      <WhenSection f={f} set={set} isNew={isNew} />
      <TakeSection f={f} set={set} />
      <WhereSection f={f} set={set} country={country} />
      <PictureSection f={f} set={set} />
      <Field
        label="Tags"
        placeholder="imax, with friends"
        autoCapitalize="none"
        value={f.tags}
        onChangeText={(tags) => set({ tags })}
        hint="Separate with commas."
      />
      <Field
        label="Notes"
        multiline
        value={f.notes}
        onChangeText={(notes) => set({ notes })}
        style={{ minHeight: 80, paddingTop: 12, textAlignVertical: 'top' }}
      />
    </>
  );
}

/**
 * At a cinema, a plan whose chain matches the cinema is picked for you (one
 * match only), until you pick or clear one yourself.
 */
function usePlanSuggestion(f: LogForm, set: (p: Partial<LogForm>) => void) {
  const subs = useSubscriptions();
  const venueId = f.venue?.id;
  // Reads the whole form, but only a change of place, cinema, plans or a
  // manual pick re-suggests: editing the price or date must not.
  const suggest = useEffectEvent(() => {
    if (f.place !== 'theater' || f.subTouched || !f.venue) return;
    const day = formDay(f);
    const plan = suggestPlan(
      (subs.data?.subscriptions ?? []).filter((s) => planOnDay(s, day)),
      f.venue,
    );
    if ((plan?.id ?? null) !== f.subId) {
      set({ subId: plan?.id ?? null, ...(plan && !f.price && !f.faceValue ? { currency: plan.currency ?? f.currency } : {}) });
    }
  });
  useEffect(() => suggest(), [f.place, f.subTouched, venueId, subs.data]);
}

/** Edit a logged watch: every field, as on the web. */
function EditWatch({ watchId }: { watchId: number }) {
  const { colors } = useTheme();
  const w = useWatch(watchId);
  if (!w.data) {
    return (
      <Sheet title="" action={null} error={w.isError ? "Couldn't load this watch. Editing needs a connection." : null}>
        <ActivityIndicator color={colors.textMuted} />
      </Sheet>
    );
  }
  return <EditForm key={watchId} entry={w.data} />;
}

function EditForm({ entry: e }: { entry: components['schemas']['WatchEntry'] }) {
  // One watch carries no title or poster; the title's own record has them.
  const t = useMyTitle(e.tmdb_kind ?? 'movie', e.tmdb_id ?? '');
  const prefs = usePreferences();
  const [f, setF] = useState<LogForm>(() => formFromEntry(e, { deviceZone: deviceTimeZone() }));
  const set = (patch: Partial<LogForm>) => setF((x) => ({ ...x, ...patch }));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  usePlanSuggestion(f, set);
  const currency = f.currency || prefs.data?.currency || currencyFor(prefs.data?.country);

  async function save() {
    setError(null);
    const target = { kind: e.tmdb_kind ?? 'movie', id: e.tmdb_id ?? '', season: e.season_number, episode: e.episode_number };
    let body: WatchInput | undefined;
    const invalid = await attempt(() => {
      body = buildInput({ ...f, currency }, target);
    }, 'Check the form.');
    if (invalid || !body) return setError(invalid);
    setBusy(true);
    const id = e.id!;
    const input = body;
    const failed = await attempt(async () => unwrap(await api.PUT('/api/v1/me/watches/{id}', { params: { path: { id } }, body: input })), "Couldn't save. Are you online?");
    if (failed) {
      setError(failed);
      setBusy(false);
      return;
    }
    void queryClient.invalidateQueries();
    void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
    router.back();
  }

  return (
    <Sheet
      title={e.title ?? t.data?.title?.title ?? ''}
      subtitle={e.subject_kind === 'episode' ? episodeCode(e.season_number, e.episode_number) : undefined}
      poster={e.poster_path ?? t.data?.title?.poster_path}
      action={<SheetAction title="Save" busy={busy} onPress={save} />}
      error={error}
    >
      <FormBody f={{ ...f, currency }} set={set} isNew={false} country={prefs.data?.country} />
    </Sheet>
  );
}

function loadDraft(key: string): LogForm | null {
  try {
    const raw = Storage.getItemSync(key);
    if (!raw) return null;
    const d = JSON.parse(raw) as LogForm;
    // Drafts from an older form shape are dropped rather than half-read.
    return d.v === FORM_VERSION && Date.now() - d.savedAt < DRAFT_TTL ? d : null;
  } catch {
    return null;
  }
}

// --- layout pieces

/**
 * The sheet: title row with Close and the primary action (always visible,
 * whatever height the sheet is pulled to), then the form.
 */
function Sheet({
  title,
  subtitle,
  poster,
  action,
  error,
  children,
}: {
  title: string;
  subtitle?: string;
  poster?: string | null;
  action: ReactNode;
  error?: string | null;
  children: ReactNode;
}) {
  const { colors, gutter, radius } = useTheme();
  const insets = useSafeAreaInsets();
  const { width } = useWindowDimensions();
  const sidePanel = useWindowClass() !== 'compact';
  const fold = useFold();
  const book = isBookSplit(fold) ? fold : null;
  if (isTabletop(fold)) {
    // Tabletop (a half-open foldable on a table): the title and artwork stand
    // up on the top half, the form lies flat below the fold.
    return (
      <View style={{ flex: 1, backgroundColor: colors.bgSurface }}>
        <View style={[styles.tableTop, { height: fold.top, paddingTop: insets.top + 8, paddingHorizontal: gutter.compact }]}>
          <Poster path={poster} title={title} width={Math.max(60, Math.min(140, Math.round((fold.top - insets.top - 24) / 1.5)))} />
          <View style={{ flex: 1, gap: 8 }}>
            <Text variant="display" numberOfLines={3}>
              {title}
            </Text>
            {subtitle ? (
              <Text variant="muted" num>
                {subtitle}
              </Text>
            ) : null}
            <View style={{ flexDirection: 'row', gap: 8 }}>
              {action}
              <Pressable
                onPress={() => router.back()}
                accessibilityRole="button"
                accessibilityLabel="Close"
                style={[styles.close, { backgroundColor: colors.bgElevated }]}
              >
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
  // The header is the scroll view's sticky first row rather than a sibling
  // above it: iOS form sheets lay out their first scroll view from the
  // sheet's top edge, which put the form over a sibling header.
  const sheet = (
    <ScrollView
      style={{ flex: 1, backgroundColor: colors.bgSurface }}
      stickyHeaderIndices={[0]}
      keyboardShouldPersistTaps="handled"
      contentContainerStyle={{ paddingBottom: insets.bottom + 24 }}
    >
      <View style={{ backgroundColor: colors.bgSurface }}>
        <View style={[styles.head, { paddingHorizontal: gutter.compact }]}>
          <Poster path={poster} title={title} width={44} />
          <View style={{ flex: 1 }}>
            <Text variant="heading" numberOfLines={2}>
              {title}
            </Text>
            {subtitle ? (
              <Text variant="muted" num>
                {subtitle}
              </Text>
            ) : null}
          </View>
          {action}
          <Pressable
            onPress={() => router.back()}
            accessibilityRole="button"
            accessibilityLabel="Close"
            style={[styles.close, { backgroundColor: colors.bgElevated }]}
          >
            <Text variant="label">✕</Text>
          </Pressable>
        </View>
        {error ? (
          <Text style={{ color: colors.deltaDown, paddingHorizontal: gutter.compact }} accessibilityLiveRegion="polite">
            {error}
          </Text>
        ) : null}
      </View>
      <View style={{ padding: gutter.compact, gap: 18 }}>{children}</View>
    </ScrollView>
  );
  if (!sidePanel) return sheet;
  // Wide windows: the form slides in beside the content, which stays visible
  // (dimmed) behind it; tapping outside closes it. On a half-open book
  // foldable it takes the whole trailing panel, clear of the fold.
  const panelWidth = book ? width - (book.left + book.width) : Math.min(480, Math.round(width * 0.5));
  return (
    <View style={styles.overlay}>
      <Pressable style={[StyleSheet.absoluteFill, { backgroundColor: 'rgba(0,0,0,0.45)' }]} onPress={() => router.back()} accessibilityLabel="Close" />
      <View
        style={[
          styles.panel,
          {
            width: panelWidth,
            paddingTop: insets.top,
            paddingRight: insets.right,
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
      {busy ? (
        <ActivityIndicator color={colors.accentOn} />
      ) : (
        <Text variant="label" style={{ color: colors.accentOn, fontSize: 16 }}>
          {title}
        </Text>
      )}
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
  venuePicked: { flexDirection: 'row', alignItems: 'center', gap: 12, padding: 12, borderRadius: 12, borderCurve: 'continuous' },
  money: { flexDirection: 'row', gap: 10 },
});
