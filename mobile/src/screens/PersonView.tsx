import { Stack } from 'expo-router';
import { useMemo, useState } from 'react';
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, View } from 'react-native';

import { ApiError } from '@/api/client';
import type { components } from '@/api/schema';
import { Chip } from '@/components/Chip';
import { PosterTile } from '@/components/Media';
import { Poster } from '@/components/Poster';
import { Panel } from '@/components/Screen';
import { Text } from '@/components/Text';
import { usePerson } from '@/data/queries';
import { openTitle, titleHref } from '@/lib/actions';
import { useTheme } from '@/theme';

import { Face } from './cast-parts';
import { MiniSegment } from './stats-parts';

type Person = components['schemas']['PersonResponse'];
type Film = NonNullable<Person['filmography']>[number];
type Season = NonNullable<Person['filmography_seasons']>[number];

// Talk shows, award ceremonies and documentaries list people "as Self";
// they swamp an actor's credits, so they are hidden unless asked for (as on
// the web's person page).
const SELF = /^as (self|himself|herself|themselves|themself)\b/i;
const selfOnly = (roles?: string[]) => !!roles?.length && roles.every((r) => SELF.test(r));

const PAGE = 40;

/**
 * A person: their photo and profile, the titles of theirs that charted, and
 * their filmography, films one per title and TV once per season (season
 * poster, episodes they are in, their role), newest first.
 */
export function PersonView({ id }: { id: number }) {
  const { colors, gutter } = useTheme();
  const person = usePerson(id);
  const p = person.data;
  const [tab, setTab] = useState<'movie' | 'tv'>('movie');
  const [self, setSelf] = useState(false);
  const [bioOpen, setBioOpen] = useState(false);
  const [limit, setLimit] = useState(PAGE);

  const films = useMemo(() => (p?.filmography ?? []).filter((f) => f.kind === 'movie' && (self || !selfOnly(f.roles))), [p, self]);
  const seasons = useMemo(() => (p?.filmography_seasons ?? []).filter((s) => self || !selfOnly(s.roles)), [p, self]);
  // Without per-season credits (TMDB failed), TV falls back to one row per show.
  const shows = useMemo(() => (p?.filmography ?? []).filter((f) => f.kind === 'tv' && (self || !selfOnly(f.roles))), [p, self]);
  const bySeason = seasons.length > 0 || p?.seasons_status === 'complete';
  const selfCount = (p?.filmography ?? []).filter((f) => selfOnly(f.roles)).length;
  const charted = (p?.titles ?? []).filter((t) => (t.days ?? 0) > 0);
  const det = p?.details;

  if (person.isPending) {
    return (
      <View style={[styles.center, { backgroundColor: colors.bgCanvas }]}>
        <Stack.Screen options={{ headerTitle: '' }} />
        <ActivityIndicator color={colors.textMuted} />
      </View>
    );
  }
  if (!p) {
    const missing = person.error instanceof ApiError && person.error.status === 404;
    return (
      <View style={[styles.center, { backgroundColor: colors.bgCanvas, padding: gutter.compact }]}>
        <Stack.Screen options={{ headerTitle: '' }} />
        <Text variant="muted">{missing ? 'Neither metareel nor TMDB knows this person.' : "Couldn't load this person. Try again later."}</Text>
      </View>
    );
  }

  const life = [
    det?.known_for ?? p.known_for,
    det?.birthday ? `Born ${longDate(det.birthday)}${det.deathday ? '' : ` (${age(det.birthday)})`}` : '',
    det?.deathday ? `Died ${longDate(det.deathday)}${det.birthday ? ` (aged ${age(det.birthday, det.deathday)})` : ''}` : '',
    det?.place_of_birth,
  ].filter(Boolean);

  return (
    <ScrollView
      style={{ backgroundColor: colors.bgCanvas }}
      contentInsetAdjustmentBehavior="automatic"
      contentContainerStyle={{ padding: gutter.compact, gap: 16, paddingBottom: 40, width: '100%', maxWidth: 760, alignSelf: 'center' }}
    >
      <Stack.Screen options={{ headerTitle: '' }} />
      <View style={styles.hero}>
        <Face path={p.profile_path} name={p.name ?? ''} size={112} />
        <View style={{ flex: 1, gap: 4 }}>
          <Text variant="display" accessibilityRole="header">
            {p.name}
          </Text>
          {life.map((l) => (
            <Text key={l} variant="muted">
              {l}
            </Text>
          ))}
        </View>
      </View>

      {det?.biography ? (
        <Pressable onPress={() => setBioOpen((o) => !o)} accessibilityRole="button" accessibilityState={{ expanded: bioOpen }} accessibilityHint="Shows the whole biography">
          <Text numberOfLines={bioOpen ? undefined : 5}>{det.biography}</Text>
          <Text variant="caption" style={{ marginTop: 4 }}>
            {bioOpen ? 'Less' : 'More'}
          </Text>
        </Pressable>
      ) : null}

      {charted.length ? (
        <Panel title="On the charts">
          <Text variant="muted" num>
            {charted.length} {charted.length === 1 ? 'title' : 'titles'} charted · {(p.points ?? 0).toLocaleString()} chart points
          </Text>
          <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 12 }}>
            {charted.slice(0, 12).map((t) => (
              <PosterTile
                key={`${t.kind}:${t.tmdb_id}`}
                poster={t.poster_path}
                title={t.title ?? ''}
                width={104}
                caption={t.best_rank ? `Best #${t.best_rank} · ${t.days} days` : `${t.days} days`}
                href={t.tmdb_id ? titleHref({ kind: t.kind === 'tv' ? 'tv' : 'movie', id: t.tmdb_id, title: t.title ?? '' }) : undefined}
              />
            ))}
          </ScrollView>
        </Panel>
      ) : null}

      {p.details_status !== 'ok' && !p.filmography?.length ? (
        <Text variant="muted">
          {p.details_status === 'unavailable' ? "TMDB couldn't be reached, so the filmography is missing. Try again later." : 'No filmography from TMDB.'}
        </Text>
      ) : (
        <View style={{ gap: 12 }}>
          <Text variant="title" accessibilityRole="header">
            Filmography
          </Text>
          <MiniSegment
            options={[
              ['movie', `Films · ${films.length}`],
              ['tv', `TV · ${bySeason ? seasons.length : shows.length}`],
            ]}
            value={tab}
            onChange={(t) => {
              setTab(t);
              setLimit(PAGE);
            }}
          />
          {selfCount ? (
            <View style={{ flexDirection: 'row' }}>
              <Chip label={self ? `Hiding no appearances` : `Show ${selfCount} appearances as themselves`} selected={self} onPress={() => setSelf((s) => !s)} />
            </View>
          ) : null}
          {tab === 'tv' && bySeason && p.seasons_status === 'partial' ? (
            <Text variant="caption">{"Some shows weren't looked up; the ones with the most episodes are listed."}</Text>
          ) : null}
          <View>
            {tab === 'movie'
              ? films.slice(0, limit).map((f) => <FilmRow key={`${f.kind}:${f.tmdb_id}`} f={f} />)
              : bySeason
                ? seasons.slice(0, limit).map((s) => <SeasonRow key={`${s.tmdb_id}:${s.season_number}`} s={s} />)
                : shows.slice(0, limit).map((f) => <FilmRow key={`${f.kind}:${f.tmdb_id}`} f={f} />)}
          </View>
          {(tab === 'movie' ? films.length : bySeason ? seasons.length : shows.length) > limit ? (
            <View style={{ flexDirection: 'row' }}>
              <Chip label="Show more" onPress={() => setLimit((l) => l + PAGE)} />
            </View>
          ) : null}
          {(tab === 'movie' ? films.length : bySeason ? seasons.length : shows.length) === 0 ? (
            <Text variant="muted">{tab === 'movie' ? 'No films.' : 'No TV.'}</Text>
          ) : null}
          <Text variant="caption">Filmography from TMDB</Text>
        </View>
      )}
    </ScrollView>
  );
}

function FilmRow({ f }: { f: Film }) {
  const { colors } = useTheme();
  const roles = (f.roles ?? []).slice(0, 2).join(' · ');
  return (
    <Pressable
      onPress={() => openTitle({ kind: f.kind === 'tv' ? 'tv' : 'movie', id: f.tmdb_id ?? '', title: f.title ?? '' })}
      accessibilityRole="button"
      accessibilityLabel={[f.title, f.year ? String(f.year) : 'not released yet', roles].filter(Boolean).join(', ')}
      style={({ pressed }) => [styles.row, { borderBottomColor: colors.borderSubtle, opacity: pressed ? 0.7 : 1 }]}
    >
      <Poster path={f.poster_path} title={f.title ?? ''} width={44} />
      <View style={{ flex: 1, gap: 2 }}>
        <Text variant="label" numberOfLines={2}>
          {f.title}
        </Text>
        <Text variant="caption" num>
          {[f.year ?? 'Upcoming', f.kind === 'tv' && f.episodes ? `${f.episodes} ${f.episodes === 1 ? 'episode' : 'episodes'}` : ''].filter(Boolean).join(' · ')}
        </Text>
        {roles ? (
          <Text variant="muted" numberOfLines={1}>
            {roles}
          </Text>
        ) : null}
      </View>
    </Pressable>
  );
}

function SeasonRow({ s }: { s: Season }) {
  const { colors } = useTheme();
  const roles = (s.roles ?? []).slice(0, 2).join(' · ');
  const name = s.season_number === 0 ? 'Specials' : s.season_name && !/^season \d+$/i.test(s.season_name) ? s.season_name : `Season ${s.season_number}`;
  const eps = s.season_episode_count ? `${s.episode_count} of ${s.season_episode_count} episodes` : `${s.episode_count} ${s.episode_count === 1 ? 'episode' : 'episodes'}`;
  return (
    <Pressable
      onPress={() => openTitle({ kind: 'tv', id: s.tmdb_id ?? '', title: s.title ?? '' })}
      accessibilityRole="button"
      accessibilityLabel={[s.title, name, s.year ? String(s.year) : 'not aired yet', eps, roles].filter(Boolean).join(', ')}
      style={({ pressed }) => [styles.row, { borderBottomColor: colors.borderSubtle, opacity: pressed ? 0.7 : 1 }]}
    >
      <Poster path={s.poster_path} title={s.title ?? ''} width={44} />
      <View style={{ flex: 1, gap: 2 }}>
        <Text variant="label" numberOfLines={1}>
          {s.title}
        </Text>
        <Text variant="caption" num numberOfLines={1}>
          {[name, s.year ?? 'Upcoming'].join(' · ')}
        </Text>
        <Text variant="muted" numberOfLines={1} num>
          {[eps, roles].filter(Boolean).join(' · ')}
        </Text>
      </View>
    </Pressable>
  );
}

function longDate(d: string): string {
  return new Date(`${d.slice(0, 10)}T12:00:00`).toLocaleDateString(undefined, { day: 'numeric', month: 'long', year: 'numeric' });
}

function age(birth: string, until?: string | null): number {
  const b = new Date(`${birth.slice(0, 10)}T12:00:00`);
  const u = until ? new Date(`${until.slice(0, 10)}T12:00:00`) : new Date();
  let a = u.getFullYear() - b.getFullYear();
  if (u.getMonth() < b.getMonth() || (u.getMonth() === b.getMonth() && u.getDate() < b.getDate())) a--;
  return a;
}

const styles = StyleSheet.create({
  center: { flex: 1, alignItems: 'center', justifyContent: 'center' },
  hero: { flexDirection: 'row', alignItems: 'center', gap: 16 },
  row: { flexDirection: 'row', alignItems: 'center', gap: 12, paddingVertical: 10, borderBottomWidth: StyleSheet.hairlineWidth },
});
