import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useMutation } from '@tanstack/react-query';
import { router } from 'expo-router';
import { ActivityIndicator, Alert, ScrollView, StyleSheet, View } from 'react-native';

import { unwrap } from '@/api/client';
import { api } from '@/auth/session';
import { Button } from '@/components/Button';
import { Panel } from '@/components/Screen';
import { Text } from '@/components/Text';
import { TitleRow } from '@/components/TitleRow';
import { useMyTitle, useWatch } from '@/data/queries';
import { openTitle } from '@/lib/actions';
import { episodeCode, formatMoney, starText } from '@/lib/format';
import { queryClient } from '@/query/client';
import { useTheme } from '@/theme';

/** One watch from the diary: what, when, where; edit or delete it. */
export function WatchView({ id, embedded }: { id: string; embedded?: boolean }) {
  const { colors, gutter } = useTheme();
  const insets = useSafeAreaInsets();
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

  const e = w.data;
  const place = e
    ? [e.venue_name_at_watch, e.venue_city_at_watch].filter(Boolean).join(', ') || e.provider_name || (e.venue_kind === 'physical' ? 'At home, on disc' : '')
    : '';
  const money = e?.price_minor != null ? formatMoney(e.price_minor, e.currency) : e?.face_value_minor != null ? `${formatMoney(e.face_value_minor, e.currency)} face value` : '';
  return (
    <ScrollView style={{ backgroundColor: colors.bgCanvas }} contentContainerStyle={{ padding: gutter.compact, paddingTop: embedded ? insets.top + 12 : gutter.compact, gap: 16 }}>
      {!e ? (
        <ActivityIndicator color={colors.textMuted} />
      ) : (
        <>
          <TitleRow
            poster={e.poster_path ?? t.data?.title?.poster_path}
            title={e.title ?? t.data?.title?.title ?? ''}
            subtitle={e.subject_kind === 'episode' ? episodeCode(e.season_number, e.episode_number) : (e.release_date ?? '').slice(0, 4)}
            onPress={() => openTitle({ kind: e.tmdb_kind ?? 'movie', id: e.tmdb_id ?? '' })}
          />
          <Panel>
            <Line label="Watched" value={new Date(e.ended_at ?? '').toLocaleString(undefined, e.time_precision === 'day' ? { dateStyle: 'medium' } : { dateStyle: 'medium', timeStyle: 'short' })} />
            {e.rating ? <Line label="Rating" value={starText(e.rating)} accent /> : null}
            {e.liked ? <Line label="Liked" value="♥" accent /> : null}
            {e.is_rewatch ? <Line label="Rewatch" value={`#${e.watch_number}`} /> : null}
            {place ? <Line label="Where" value={place} /> : null}
            {e.formats?.length ? <Line label="Format" value={e.formats.map((f) => f.label).join(', ')} /> : null}
            {money ? <Line label="Paid" value={money} /> : null}
            {e.notes ? <Text style={{ marginTop: 6 }}>{e.notes}</Text> : null}
          </Panel>
          <View style={styles.buttons}>
            <Button title="Edit" kind="quiet" onPress={() => router.push({ pathname: '/log', params: { kind: e.tmdb_kind ?? 'movie', id: e.tmdb_id ?? '', watchId: String(e.id) } })} />
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

const styles = StyleSheet.create({
  line: { flexDirection: 'row', gap: 12, minHeight: 26, alignItems: 'center' },
  buttons: { gap: 10 },
});
