import { Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, View } from 'react-native';

import { Text } from '@/components/Text';
import { useDetailedStats } from '@/data/queries';
import { openTitle } from '@/lib/actions';
import { starText } from '@/lib/format';
import { usePersisted } from '@/lib/persisted';
import { RatingsChart } from '@/screens/diary-parts';
import { BarChart, BarList, Card, Duration, hours, MiniSegment, PeopleList, RankList, shortDuration } from '@/screens/stats-parts';
import { useTheme } from '@/theme';

type Kind = 'tv' | 'movie';
type Period = 'weeks' | 'months' | 'years';

const shortMonth = (ym: string) => new Date(`${ym}-15T12:00:00`).toLocaleDateString(undefined, { month: 'short' });
const shortDay = (d: string) => new Date(`${d}T12:00:00`).toLocaleDateString(undefined, { month: 'numeric', day: 'numeric' });

/**
 * Detailed statistics, shows or films: time spent, counts, the titles you
 * spent most time on, where your shows stand, what's left and when you'd
 * catch up, trends, the day of the week, ratings, genres, people and binges.
 */
export default function Stats() {
  const params = useLocalSearchParams<{ kind?: string }>();
  const [saved, setSaved] = usePersisted<Kind>('stats.kind', 'tv', ['tv', 'movie']);
  const [kind, setKindState] = useState<Kind>(params.kind === 'movie' || params.kind === 'tv' ? params.kind : saved);
  const setKind = (k: Kind) => {
    setKindState(k);
    setSaved(k);
  };
  const { colors, gutter } = useTheme();
  const q = useDetailedStats(kind);
  const s = q.data;
  const [period, setPeriod] = useState<Period>('weeks');
  const [measure, setMeasure] = useState<'time' | 'count'>('time');
  const tv = kind === 'tv';
  const noun = tv ? 'episodes' : 'movies';
  const nounOf = (n: number) => (n === 1 ? (tv ? 'episode' : 'movie') : noun);

  const trend = s ? ((period === 'weeks' ? s.trend?.weeks : period === 'months' ? s.trend?.months : s.trend?.years) ?? []) : [];
  const bars = trend.map((p) => ({
    label: period === 'weeks' ? shortDay(p.label ?? '') : period === 'months' ? shortMonth(p.label ?? '') : (p.label ?? ''),
    value: measure === 'time' ? (p.minutes ?? 0) : (p.watches ?? 0),
  }));
  const fmt = (n: number) => (measure === 'time' ? hours(n) : `${n.toLocaleString()} ${nounOf(n)}`);

  return (
    <View style={{ flex: 1, backgroundColor: colors.bgCanvas }}>
      <Stack.Screen options={{ headerTitle: 'Detailed stats' }} />
      <View style={{ paddingHorizontal: gutter.compact }}>
        <View style={[styles.segments, { borderBottomColor: colors.borderSubtle, width: '100%', maxWidth: 760 - 2 * gutter.compact, alignSelf: 'center' }]}>
          {(
            [
              ['tv', 'Shows'],
              ['movie', 'Movies'],
            ] as [Kind, string][]
          ).map(([k, label]) => (
            <Pressable
              key={k}
              onPress={() => setKind(k)}
              accessibilityRole="tab"
              accessibilityState={{ selected: kind === k }}
              style={[styles.segment, { borderBottomColor: kind === k ? colors.textPrimary : 'transparent' }]}
            >
              <Text
                variant="label"
                style={{ color: kind === k ? colors.textPrimary : colors.textMuted, fontSize: 15, letterSpacing: 0.4, textTransform: 'uppercase' }}
              >
                {label}
              </Text>
            </Pressable>
          ))}
        </View>
      </View>
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        contentContainerStyle={{ padding: gutter.compact, gap: 12, paddingBottom: 40, width: '100%', maxWidth: 760, alignSelf: 'center' }}
      >
        {q.isPending ? <ActivityIndicator color={colors.textMuted} /> : null}
        {q.isError ? <Text variant="muted">Couldn&apos;t load your statistics. They need a connection.</Text> : null}
        {s ? (
          <>
            <Card title={tv ? 'Time spent watching series' : 'Time spent watching movies'}>
              <Duration minutes={s.minutes ?? 0} />
            </Card>
            <View style={styles.pair}>
              <Card flex title={tv ? 'Watched episodes' : 'Movies watched'}>
                <Big value={(tv ? s.episodes : s.movies) ?? 0} />
              </Card>
              <Card flex title={tv ? 'Rewatched episodes' : 'Rewatches'}>
                <Big value={s.rewatches ?? 0} />
              </Card>
            </View>
            <View style={styles.pair}>
              <Card flex title={tv ? 'Shows' : 'Films'}>
                <Big value={(tv ? s.shows : s.films) ?? 0} />
              </Card>
              <Card flex title="Streak">
                <Text variant="display" num>
                  {s.streak?.current ?? 0}
                  <Text variant="muted">
                    {' '}
                    {s.streak?.current === 1 ? 'day' : 'days'} · best {s.streak?.longest ?? 0}
                  </Text>
                </Text>
              </Card>
            </View>

            {s.top_titles?.length ? (
              <Card title={tv ? 'Most-watched series' : 'Most-watched movies'}>
                <RankList
                  rows={s.top_titles.map((t) => ({
                    key: `${t.tmdb_kind}-${t.tmdb_id}`,
                    title: t.title ?? '',
                    detail: tv ? `${t.watches} ${t.watches === 1 ? 'episode' : 'episodes'}` : t.watches && t.watches > 1 ? `${t.watches} times` : undefined,
                    value: hours(t.minutes ?? 0),
                    onPress: () => openTitle({ kind: tv ? 'tv' : 'movie', id: t.tmdb_id ?? '', title: t.title }),
                  }))}
                />
              </Card>
            ) : null}

            {tv && s.shows_by_stage?.length ? (
              <Card title="Breakdown by status">
                <BarList rows={s.shows_by_stage.map((x) => ({ label: x.label ?? '', value: x.count ?? 0 }))} />
              </Card>
            ) : null}

            {tv && s.minutes_left != null ? (
              <Card title="Time left to watch">
                <Duration minutes={s.minutes_left} />
                <Text variant="caption">Aired episodes you haven&apos;t seen of the shows you&apos;re on</Text>
              </Card>
            ) : null}

            {tv && s.catch_up_date ? (
              <Card title="Catch-up date">
                <Text variant="display" style={{ color: colors.accentYours }}>
                  {new Date(`${s.catch_up_date}T12:00:00`).toLocaleDateString(undefined, { day: 'numeric', month: 'long', year: 'numeric' })}
                </Text>
                <Text variant="caption">At your pace of the last 8 weeks ({shortDuration(s.pace_minutes_per_week ?? 0)} a week)</Text>
              </Card>
            ) : null}

            {tv && s.upcoming_by_month?.some((m) => m.watches) ? (
              <Card title="Upcoming watch time">
                {s.upcoming_by_month.some((m) => m.minutes) ? (
                  <BarChart
                    bars={s.upcoming_by_month.map((m) => ({ label: shortMonth(m.label ?? ''), value: m.minutes ?? 0 }))}
                    format={(n) => hours(n)}
                    height={110}
                  />
                ) : (
                  // Runtimes of unaired episodes are often unknown yet: count them instead.
                  <BarChart
                    bars={s.upcoming_by_month.map((m) => ({ label: shortMonth(m.label ?? ''), value: m.watches ?? 0 }))}
                    format={(n) => `${n} ${n === 1 ? 'episode' : 'episodes'}`}
                    height={110}
                  />
                )}
              </Card>
            ) : null}

            <View style={styles.pair}>
              <Card flex title="This week">
                <Text variant="title" num>
                  {(s.this_week?.watches ?? 0).toLocaleString()} <Text variant="muted">{nounOf(s.this_week?.watches ?? 0)}</Text>
                </Text>
                <Text variant="muted" num>
                  {shortDuration(s.this_week?.minutes ?? 0)}
                </Text>
              </Card>
              <Card flex title="This month">
                <Text variant="title" num>
                  {(s.this_month?.watches ?? 0).toLocaleString()} <Text variant="muted">{nounOf(s.this_month?.watches ?? 0)}</Text>
                </Text>
                <Text variant="muted" num>
                  {shortDuration(s.this_month?.minutes ?? 0)}
                </Text>
              </Card>
            </View>

            <Card title="Trend">
              <MiniSegment
                options={[
                  ['time', 'Watch time'],
                  ['count', tv ? 'Episodes' : 'Movies'],
                ]}
                value={measure}
                onChange={setMeasure}
              />
              <MiniSegment
                options={[
                  ['weeks', 'Weeks'],
                  ['months', 'Months'],
                  ['years', 'Years'],
                ]}
                value={period}
                onChange={setPeriod}
              />
              <BarChart key={`${kind}-${period}-${measure}`} bars={bars} format={fmt} />
            </Card>

            <Card title="Day of the week">
              <BarChart
                key={`${kind}-weekday-${measure}`}
                bars={(s.by_weekday ?? []).map((d) => ({
                  label: (d.label ?? '').slice(0, 3),
                  value: measure === 'time' ? (d.minutes ?? 0) : (d.watches ?? 0),
                }))}
                format={fmt}
                height={110}
              />
            </Card>

            <Card title="Ratings">
              {s.ratings?.rated ? (
                <>
                  <Text>
                    Average <Text style={{ color: colors.accentYours }}>{starText(Math.round(s.ratings.average ?? 0))}</Text>
                    <Text variant="muted" num>
                      {' '}
                      ({((s.ratings.average ?? 0) / 2).toFixed(1)} of 5)
                    </Text>
                  </Text>
                  <RatingsChart
                    buckets={Object.entries(s.ratings.distribution ?? {}).map(([r, n]) => ({ rating: Number(r), watches: n }))}
                    rated={s.ratings.rated}
                    average={s.ratings.average}
                  />
                </>
              ) : (
                <Text variant="muted">No ratings yet</Text>
              )}
            </Card>

            {s.genres?.length ? (
              <Card title="Genres">
                <BarList rows={s.genres.map((g) => ({ label: g.label ?? '', value: g.count ?? 0 }))} />
              </Card>
            ) : null}

            {s.actors?.length ? (
              <Card title="Most-watched actors">
                <PeopleList
                  people={s.actors}
                  unit={(n) => `${n.toLocaleString()} ${tv ? (n === 1 ? 'episode' : 'episodes') : n === 1 ? 'viewing' : 'viewings'}`}
                />
              </Card>
            ) : null}
            {s.directors?.length ? (
              <Card title="Most-watched directors">
                <PeopleList people={s.directors} unit={(n) => `${n.toLocaleString()} ${n === 1 ? 'viewing' : 'viewings'}`} />
              </Card>
            ) : null}

            {tv && s.binges?.length ? (
              <Card title="Biggest binges">
                <RankList
                  rows={s.binges.map((b) => ({
                    key: `${b.tmdb_id}-${b.day}`,
                    title: b.title ?? '',
                    detail: `${new Date(`${b.day}T12:00:00`).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' })} · ${hours(b.minutes ?? 0)}`,
                    value: `${b.episodes} ep.`,
                    onPress: () => openTitle({ kind: 'tv', id: b.tmdb_id ?? '', title: b.title }),
                  }))}
                />
              </Card>
            ) : null}
          </>
        ) : null}
      </ScrollView>
    </View>
  );
}

function Big({ value }: { value: number }) {
  return (
    <Text variant="display" num style={{ fontSize: 34, lineHeight: 40 }}>
      {value.toLocaleString()}
    </Text>
  );
}

const styles = StyleSheet.create({
  segments: { flexDirection: 'row', borderBottomWidth: StyleSheet.hairlineWidth },
  segment: { flex: 1, alignItems: 'center', paddingVertical: 10, borderBottomWidth: 2, marginBottom: -StyleSheet.hairlineWidth },
  pair: { flexDirection: 'row', gap: 12 },
});
