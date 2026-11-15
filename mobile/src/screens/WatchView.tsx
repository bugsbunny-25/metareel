import { useDetailScroll } from '@/layout/root-header';
import { useMutation } from '@tanstack/react-query';
import { router } from 'expo-router';
import { ActivityIndicator, Alert, ScrollView, StyleSheet, View } from 'react-native';

import { unwrap } from '@/api/client';
import type { components } from '@/api/schema';
import { api } from '@/auth/session';
import { Button } from '@/components/Button';
import { Panel } from '@/components/Screen';
import { Text } from '@/components/Text';
import { TitleRow } from '@/components/TitleRow';
import { FormatLogo } from '@/components/FormatLogo';
import { useMyTitle, useSubscriptions, useWatch } from '@/data/queries';
import { openEpisode, openTitle } from '@/lib/actions';
import { episodeCode, formatMoney, starText } from '@/lib/format';
import { queryClient } from '@/query/client';
import { fromUTC } from '@/log/zones';
import { DIMENSIONS, HDRS, OTHER_MEDIA, PLANS, RESOLUTIONS, SCREENINGS } from '@/log/form';
import { useTheme } from '@/theme';

type Format = NonNullable<components['schemas']['WatchEntry']['formats']>[number];

const label = (list: readonly (readonly [string, string])[], v?: string | null) => (v ? (list.find(([k]) => k === v)?.[1] ?? v) : '');

/**
 * When a watch happened, in the zone it was filed in (a cinema abroad shows
 * its local time), as precisely as it was logged.
 */
export function watchedText(e: { ended_at?: string; tz?: string | null; time_precision?: string; watched_on?: string }): string {
  if (e.time_precision === 'year') return (e.watched_on ?? '').slice(0, 4);
  if (e.time_precision === 'month' && e.watched_on)
    return new Date(`${e.watched_on}T12:00:00`).toLocaleDateString(undefined, { month: 'long', year: 'numeric' });
  if (e.time_precision === 'day' && e.watched_on) return new Date(`${e.watched_on}T12:00:00`).toLocaleDateString(undefined, { dateStyle: 'medium' });
  const d = fromUTC(e.ended_at);
  if (!d) return '';
  const opts: Intl.DateTimeFormatOptions = { dateStyle: 'medium', timeStyle: 'short' };
  try {
    return d.toLocaleString(undefined, e.tz ? { ...opts, timeZone: e.tz } : opts);
  } catch {
    return d.toLocaleString(undefined, opts);
  }
}

/** One watch from the diary: what, when, where; edit or delete it. */
export function WatchView({ id, embedded }: { id: string; embedded?: boolean }) {
  const { colors, gutter } = useTheme();
  const detail = useDetailScroll(embedded);
  const w = useWatch(Number(id));
  // One watch carries no title or poster (the diary list joins them in).
  const t = useMyTitle(w.data?.tmdb_kind ?? 'movie', w.data?.tmdb_id ?? '', !!w.data);
  const del = useMutation({
    mutationFn: async () => unwrap(await api.DELETE('/api/v1/me/watches/{id}', { params: { path: { id: Number(id) } } })),
    onSuccess: () => {
      void queryClient.invalidateQueries();
      router.back();
    },
    onError: () => Alert.alert("Couldn't delete", 'Deleting needs a connection. Try again when you are online.'),
  });

  const subs = useSubscriptions();
  const e = w.data;
  const place = e
    ? [e.venue_name_at_watch, e.venue_city_at_watch].filter(Boolean).join(', ') ||
      (e.venue_kind === 'physical' ? 'At home' : e.venue_kind === 'other' ? 'Elsewhere' : '')
    : '';
  const plan = e?.subscription_id ? (subs.data?.subscriptions ?? []).find((s) => s.id === e.subscription_id)?.plan : null;
  const money = (m?: number | null) => (m != null ? formatMoney(m, e?.currency) : '');
  const formats = e?.formats ?? [];
  const cinemaFormats = formats.filter((f) => f.category !== 'disc' && f.category !== 'label');
  const discs = formats.filter((f) => f.category === 'disc');
  const labels = formats.filter((f) => f.category === 'label');
  const screening = e?.screening_type && e.screening_type !== 'regular' ? [label(SCREENINGS, e.screening_type), e.event_name].filter(Boolean).join(' · ') : '';
  const picture = e
    ? [
        label(DIMENSIONS, e.dimension),
        label(RESOLUTIONS, e.resolution),
        e.hdr && e.hdr !== 'none' ? label(HDRS, e.hdr) : '',
        e.frame_rate ? `${e.frame_rate} fps` : '',
      ]
        .filter(Boolean)
        .join(' · ')
    : '';
  const aspect = e ? [e.content_aspect_ratio_text, e.content_aspect_variable ? 'variable' : ''].filter(Boolean).join(', ') : '';
  const languages = e ? [e.audio_language, e.subtitle_language ? `${e.subtitle_language} subtitles` : ''].filter(Boolean).join(' · ') : '';
  return (
    <ScrollView
      style={{ backgroundColor: colors.bgCanvas }}
      contentInsetAdjustmentBehavior={detail.contentInsetAdjustmentBehavior}
      contentContainerStyle={{ padding: gutter.compact, paddingTop: detail.paddingTop ?? gutter.compact, gap: 16 }}
    >
      {!e ? (
        <ActivityIndicator color={colors.textMuted} />
      ) : (
        <>
          <TitleRow
            poster={e.poster_path ?? t.data?.title?.poster_path}
            title={e.title ?? t.data?.title?.title ?? ''}
            kind={e.tmdb_kind}
            tmdbId={e.tmdb_id}
            subtitle={e.subject_kind === 'episode' ? episodeCode(e.season_number, e.episode_number) : (e.release_date ?? '').slice(0, 4)}
            onPress={() =>
              e.subject_kind === 'episode' && e.season_number != null && e.episode_number != null
                ? openEpisode(e.tmdb_id ?? '', e.season_number, e.episode_number)
                : openTitle({ kind: e.tmdb_kind ?? 'movie', id: e.tmdb_id ?? '' })
            }
          />
          <Panel>
            <Line label="Watched" value={watchedText(e)} />
            {e.rating ? <Line label="Rating" value={starText(e.rating)} accent /> : null}
            {e.liked ? <Line label="Liked" value="♥" accent /> : null}
            {e.is_rewatch ? <Line label="Rewatch" value={`#${e.watch_number}`} /> : null}
            {place ? <Line label="Where" value={place} /> : null}
            {e.seat ? <Line label="Seat" value={e.seat} /> : null}
            {screening ? <Line label="Screening" value={screening} /> : null}
            {e.with_qa ? <Line label="Q&A" value="With a Q&A or introduction" /> : null}
            {cinemaFormats.length ? <Badges label="Format" formats={cinemaFormats} /> : null}
            {e.provider_name ? <Line label={e.venue_kind === 'broadcast' ? 'Channel' : 'Service'} value={e.provider_name} /> : null}
            {e.monetization ? <Line label="Plan" value={label(PLANS, e.monetization)} /> : null}
            {discs.length ? <Badges label="Disc" formats={discs} named /> : e.media ? <Line label="Media" value={label(OTHER_MEDIA, e.media)} /> : null}
            {labels.length ? <Badges label="Label" formats={labels} named /> : null}
            {e.edition ? <Line label="Edition" value={e.edition} /> : null}
            {e.device ? <Line label="Device" value={e.device} /> : null}
            {plan ? <Line label="Covered by" value={plan} accent /> : null}
            {e.face_value_minor != null ? <Line label="Face value" value={money(e.face_value_minor)} /> : null}
            {e.price_minor != null ? <Line label={plan ? 'Paid on top' : 'Paid'} value={money(e.price_minor)} /> : null}
            {e.fees_minor ? <Line label="Booking fees" value={money(e.fees_minor)} /> : null}
            {e.concessions_minor ? <Line label="Snacks" value={money(e.concessions_minor)} /> : null}
            {e.tickets_count && e.tickets_count > 1 ? <Line label="Tickets" value={String(e.tickets_count)} /> : null}
            {e.booking_source ? <Line label="Booked through" value={e.booking_source} /> : null}
            {picture ? <Line label="Picture" value={picture} /> : null}
            {e.audio_format ? <Line label="Sound" value={e.audio_format} /> : null}
            {languages ? <Line label="Language" value={languages} /> : null}
            {aspect ? <Line label="Aspect ratio" value={aspect} /> : null}
            {e.tags?.length ? <Line label="Tags" value={e.tags.join(', ')} /> : null}
            {e.notes ? <Text style={{ marginTop: 6 }}>{e.notes}</Text> : null}
          </Panel>
          <View style={styles.buttons}>
            <Button
              title="Edit"
              kind="quiet"
              onPress={() => router.push({ pathname: '/log', params: { kind: e.tmdb_kind ?? 'movie', id: e.tmdb_id ?? '', watchId: String(e.id) } })}
            />
            <Button
              title="Delete"
              kind="quiet"
              busy={del.isPending}
              onPress={() =>
                Alert.alert('Delete this watch?', 'It is removed from your diary for good.', [
                  { text: 'Cancel', style: 'cancel' },
                  { text: 'Delete', style: 'destructive', onPress: () => del.mutate() },
                ])
              }
            />
          </View>
        </>
      )}
    </ScrollView>
  );
}

function Line({ label, value, accent }: { label: string; value: string; accent?: boolean }) {
  const { colors } = useTheme();
  return (
    <View style={styles.line}>
      <Text variant="muted">{label}</Text>
      <Text style={[{ flex: 1, textAlign: 'right' }, accent && { color: colors.accentYours }]}>{value}</Text>
    </View>
  );
}

/**
 * Formats as their logos on dark plates (the label when there is no logo),
 * like the web's FormatBadge; a disc or label also gets its name, since
 * those logos (a Criterion "C") don't say it.
 */
function Badges({ label: name, formats, named }: { label: string; formats: Format[]; named?: boolean }) {
  const { colors } = useTheme();
  return (
    <View style={styles.line}>
      <Text variant="muted">{name}</Text>
      <View style={styles.badges} accessible accessibilityLabel={`${name}: ${formats.map((f) => f.label).join(', ')}`}>
        {formats.map((f) =>
          f.logo_url && named ? (
            <View key={f.slug} style={styles.named}>
              <FormatLogo url={f.logo_url} mono={f.logo_display === 'mono'} height={18} maxWidth={64} />
              <Text>{f.label}</Text>
            </View>
          ) : f.logo_url ? (
            <FormatLogo key={f.slug} url={f.logo_url} mono={f.logo_display === 'mono'} height={18} maxWidth={84} />
          ) : (
            <View key={f.slug} style={[styles.textBadge, { backgroundColor: colors.bgElevated }]}>
              <Text variant="label" style={{ fontSize: 13 }}>
                {f.label}
              </Text>
            </View>
          ),
        )}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  badges: { flex: 1, flexDirection: 'row', flexWrap: 'wrap', justifyContent: 'flex-end', gap: 6 },
  named: { flexDirection: 'row', alignItems: 'center', gap: 8 },
  textBadge: { paddingHorizontal: 8, height: 26, borderRadius: 8, justifyContent: 'center' },
  line: { flexDirection: 'row', gap: 12, minHeight: 30, alignItems: 'center' },
  buttons: { gap: 10 },
});
