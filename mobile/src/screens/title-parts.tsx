import { Image } from 'expo-image';
import { type ReactNode, useState } from 'react';
import { Linking, Pressable, StyleSheet, View } from 'react-native';

import type { components } from '@/api/schema';
import { Poster } from '@/components/Poster';
import { RatingLogo } from '@/components/RatingLogo';
import { Top10Badge } from '@/components/Top10Badge';
import { Panel } from '@/components/Screen';
import { Text } from '@/components/Text';
import { usePreferences, useTitleAvailability } from '@/data/queries';
import { providerName } from '@/data/scope';
import { useTop10Of } from '@/data/top10now';
import { useWindowClass } from '@/layout/window-class';
import { runtimeText, tmdbImage } from '@/lib/format';
import { MiniSegment } from '@/screens/stats-parts';
import { useTheme } from '@/theme';

// The sections a film's page and a show's page share: the artwork-tinted
// hero, scores, where to watch, today's chart positions and the facts.

type Overview = components['schemas']['TitleOverview'];
type Metadata = components['schemas']['TmdbMetadata'];
type Provider = components['schemas']['WatchProvider'];

export function Hero({
  poster,
  title,
  tint,
  meta,
  children,
  kind,
  tmdbId,
  logo,
  certification,
}: {
  poster?: string | null;
  title: string;
  /** The title's logo (TMDB path): shown for the name on dark themes. */
  logo?: string | null;
  /** The age rating in your country, as a badge before the meta line. */
  certification?: string | null;
  tint?: string | null;
  meta: string;
  children?: ReactNode;
  /** The title, for the Top 10 badge and "No. 2 in films today" when it charts. */
  kind?: string;
  tmdbId?: string;
}) {
  const { colors, radius, scheme } = useTheme();
  const top = useTop10Of(kind, tmdbId);
  const wide = useWindowClass() !== 'compact';
  // TMDB's logos are mostly light artwork made for dark backdrops, so a light
  // theme keeps the typeset name.
  const logoUri = scheme === 'dark' ? tmdbImage(logo, 'w500') : null;
  return (
    <View style={[styles.hero, wide && styles.heroWide, { backgroundColor: colors.bgSurface, borderRadius: radius.hero }]}>
      {tint ? <View style={[StyleSheet.absoluteFill, { backgroundColor: tint, opacity: 0.4, borderRadius: radius.hero }]} /> : null}
      <Poster path={poster} title={title} width={wide ? 180 : 132} />
      <View style={styles.heroText}>
        {top ? (
          <View style={styles.top10} accessible accessibilityLabel={`Number ${top.rank} in ${top.category === 'tv_shows' ? 'shows' : 'films'} today on ${providerName(top.provider)}`}>
            <Top10Badge size={22} />
            <Text variant="label">No. {top.rank} in {top.category === 'tv_shows' ? 'shows' : 'films'} today</Text>
            <Text variant="muted">on {providerName(top.provider)}</Text>
          </View>
        ) : null}
        {logoUri ? (
          <View accessible accessibilityRole="header" accessibilityLabel={title} style={styles.logoBox}>
            <Image source={{ uri: logoUri }} style={StyleSheet.absoluteFill} contentFit="contain" contentPosition="left" transition={150} />
          </View>
        ) : (
          <Text variant="display" accessibilityRole="header">
            {title || ' '}
          </Text>
        )}
        {meta || certification ? (
          <View style={styles.metaRow}>
            {certification ? (
              <View style={[styles.cert, { borderColor: colors.textMuted }]} accessible accessibilityLabel={`Rated ${certification}`}>
                <Text variant="caption" num style={{ color: colors.textPrimary }}>
                  {certification}
                </Text>
              </View>
            ) : null}
            {meta ? (
              <Text variant="muted" num style={{ flexShrink: 1 }}>
                {meta}
              </Text>
            ) : null}
          </View>
        ) : null}
        {children}
      </View>
    </View>
  );
}

/**
 * The scores, each opening its source: IMDb's and Rotten Tomatoes' title
 * pages (their apps take the link when installed) and TMDB's.
 */
export function Scores({ overview }: { overview?: Overview }) {
  const r = overview?.ratings;
  const m = overview?.metadata;
  if (!r || !(r.imdb || r.rotten_tomatoes || m?.vote_average)) return null;
  const imdbUrl = r.imdb?.url ?? (r.imdb_id ? `https://www.imdb.com/title/${r.imdb_id}/` : null);
  const rtUrl = r.rotten_tomatoes?.url ?? null;
  const tmdbUrl = overview?.kind && overview.tmdb_id ? `https://www.themoviedb.org/${overview.kind}/${overview.tmdb_id}` : null;
  return (
    <View style={styles.scores}>
      {r.imdb ? <Score label="IMDb" value={r.imdb.rating.toFixed(1)} logo={<RatingLogo source="imdb" />} url={imdbUrl} /> : null}
      {r.rotten_tomatoes?.critics_score != null ? (
        <Score
          label={r.rotten_tomatoes.certified_fresh ? 'Tomatometer, Certified Fresh' : 'Tomatometer'}
          value={`${r.rotten_tomatoes.critics_score}%`}
          logo={<RatingLogo source="rt" rotten={r.rotten_tomatoes.critics_score < 60} certified={!!r.rotten_tomatoes.certified_fresh} />}
          url={rtUrl}
        />
      ) : null}
      {r.rotten_tomatoes?.audience_score != null ? (
        <Score
          label="Popcornmeter"
          value={`${r.rotten_tomatoes.audience_score}%`}
          logo={<RatingLogo source="rt-audience" rotten={r.rotten_tomatoes.audience_score < 60} />}
          url={rtUrl}
        />
      ) : null}
      {m?.vote_average ? <Score label="TMDB" value={m.vote_average.toFixed(1)} logo={<RatingLogo source="tmdb" />} url={tmdbUrl} /> : null}
    </View>
  );
}

function Score({ label, value, logo, url }: { label: string; value: string; logo: ReactNode; url?: string | null }) {
  const { colors, radius } = useTheme();
  return (
    <Pressable
      disabled={!url}
      onPress={() => url && void Linking.openURL(url)}
      accessibilityRole={url ? 'link' : undefined}
      accessibilityLabel={`${label} ${value}`}
      accessibilityHint={url ? `Opens ${label === 'TMDB' ? 'TMDB' : label.startsWith('IMDb') ? 'IMDb' : 'Rotten Tomatoes'}` : undefined}
      style={({ pressed }) => [styles.score, { backgroundColor: colors.bgSurface, borderRadius: radius.panel, opacity: pressed ? 0.7 : 1 }]}
    >
      {logo}
      <Text variant="title" num numberOfLines={1} adjustsFontSizeToFit>
        {value}
      </Text>
    </Pressable>
  );
}

/**
 * Where to watch, in your default country or any country in your order
 * (Settings), switched here. The default's providers come with the overview;
 * another country's are asked for when picked.
 */
export function WhereToWatch({ overview, country, kind, id }: { overview?: Overview; country?: string; kind: 'movie' | 'tv'; id: string }) {
  const { colors } = useTheme();
  const prefs = usePreferences();
  const options = [...new Set([country, ...(prefs.data?.countries ?? [])].filter((c): c is string => !!c))];
  const [picked, setPicked] = useState<string>();
  const where = picked && options.includes(picked) ? picked : country;
  const other = useTitleAvailability(kind, id, where, where !== country);
  const avail =
    where !== country
      ? other.data?.countries?.[where ?? '']
      : (overview?.availability?.countries?.[country ?? ''] ?? Object.values(overview?.availability?.countries ?? {})[0]);
  const streams: Provider[] = [...(avail?.flatrate ?? []), ...(avail?.free ?? []), ...(avail?.ads ?? [])];
  const buy: Provider[] = [...(avail?.rent ?? []), ...(avail?.buy ?? [])].filter((p, i, all) => all.findIndex((x) => x.provider_id === p.provider_id) === i);
  const switchable = options.length > 1 && !!overview?.availability;
  if (!streams.length && !buy.length && !switchable) return null;
  const row = (list: Provider[]) => (
    <View style={styles.providers}>
      {list.map((p) => (
        <Pressable
          key={`${p.provider_id}-${p.monetization}`}
          accessibilityRole={p.url || p.link ? 'link' : undefined}
          accessibilityLabel={`${p.provider_name}${p.monetization === 'flatrate' ? '' : `, ${p.monetization}`}`}
          onPress={() => (p.url || p.link ? void Linking.openURL((p.url || p.link)!) : undefined)}
          style={styles.provider}
        >
          {p.logo_path ? (
            <Image source={{ uri: tmdbImage(p.logo_path, 'w92')! }} style={[styles.logo, { borderColor: colors.posterHairline }]} />
          ) : (
            <View style={[styles.logo, { backgroundColor: colors.bgElevated }]} />
          )}
          <Text variant="caption" numberOfLines={1} style={{ maxWidth: 72, textAlign: 'center' }}>
            {p.provider_name}
          </Text>
        </Pressable>
      ))}
    </View>
  );
  return (
    <Panel title={switchable ? 'Where to watch' : `Where to watch${country ? ` in ${country}` : ''}`}>
      {switchable && where ? <MiniSegment options={options.map((c) => [c, c] as [string, string])} value={where} onChange={setPicked} /> : null}
      {where !== country && other.isPending ? (
        <Text variant="muted">Loading…</Text>
      ) : streams.length ? (
        row(streams)
      ) : (
        <Text variant="muted">Not streaming in {where ?? 'this country'}.</Text>
      )}
      {buy.length ? (
        <>
          <Text variant="caption">Rent or buy</Text>
          {row(buy)}
        </>
      ) : null}
      <Text variant="caption">Data from JustWatch</Text>
    </Panel>
  );
}

export function Charting({ overview }: { overview?: Overview }) {
  const now = overview?.current_positions ?? [];
  const stats = overview?.stats;
  if (!now.length && !stats?.days_on_charts) return null;
  return (
    <Panel title="On the charts">
      {now.length ? <Text variant="caption">Today</Text> : null}
      {now.slice(0, 6).map((c) => (
        <Text key={`${c.country}-${c.provider}-${c.category}`}>
          <Text variant="label" num>
            #{c.rank}
          </Text>{' '}
          {providerName(c.provider ?? '')} · {c.country}
        </Text>
      ))}
      {stats ? <ChartStats stats={stats} /> : null}
    </Panel>
  );
}

function ChartStats({ stats: s }: { stats: NonNullable<Overview['stats']> }) {
  const facts: [string, string][] = [];
  if (s.peak_rank) facts.push(['Peak', `#${s.peak_rank}${s.days_at_number_1 ? ` · ${s.days_at_number_1} days at #1` : ''}`]);
  if (s.days_on_charts) facts.push(['Days charting', String(s.days_on_charts)]);
  if (s.longest_streak_days) facts.push(['Longest run', `${s.longest_streak_days} days`]);
  if (s.countries?.length) facts.push(['Countries', String(s.countries.length)]);
  if (s.debut?.date) facts.push(['Debut', `${new Date(`${s.debut.date}T12:00:00`).toLocaleDateString()}${s.debut.country ? ` · ${s.debut.country}` : ''}`]);
  if (!facts.length) return null;
  return <Facts rows={facts} />;
}

/** The facts and the synopsis. */
export function About({ metadata: m, kind }: { metadata?: Metadata; kind: 'movie' | 'tv' }) {
  if (!m) return null;
  const date = (d?: string | null) =>
    d ? new Date(`${d.slice(0, 10)}T12:00:00`).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' }) : '';
  const rows: [string, string][] = [];
  if (kind === 'tv') {
    if (m.status) rows.push(['Status', m.status]);
    if (m.networks?.length) rows.push([m.networks.length > 1 ? 'Networks' : 'Network', m.networks.join(', ')]);
    if (m.release_date) rows.push(['First aired', date(m.release_date)]);
    if (m.last_air_date) rows.push(['Last aired', date(m.last_air_date)]);
    if (m.number_of_seasons) rows.push(['Seasons', String(m.number_of_seasons)]);
    if (m.number_of_episodes) rows.push(['Episodes', String(m.number_of_episodes)]);
    if (m.runtime) rows.push(['Episode length', runtimeText(m.runtime)]);
  } else {
    if (m.release_date) rows.push(['Released', date(m.release_date)]);
    if (m.runtime) rows.push(['Runtime', runtimeText(m.runtime)]);
    if (m.status && m.status !== 'Released') rows.push(['Status', m.status]);
  }
  if (m.genres?.length) rows.push(['Genres', m.genres.join(', ')]);
  if (m.original_language) rows.push(['Language', m.original_language.toUpperCase()]);
  if (m.origin_countries?.length) rows.push(['Country', m.origin_countries.join(', ')]);
  if (m.production_companies?.length) rows.push(['Studios', m.production_companies.slice(0, 3).join(', ')]);
  if (!m.overview && !rows.length) return null;
  return (
    <Panel title="About">
      {m.overview ? <Text>{m.overview}</Text> : null}
      {rows.length ? <Facts rows={rows} /> : null}
    </Panel>
  );
}

function Facts({ rows }: { rows: [string, string][] }) {
  return (
    <View style={{ gap: 6 }}>
      {rows.map(([k, v]) => (
        <View key={k} style={styles.fact}>
          <Text variant="caption" style={{ width: 110 }}>
            {k}
          </Text>
          <Text variant="body" style={{ flex: 1 }} num>
            {v}
          </Text>
        </View>
      ))}
    </View>
  );
}

const styles = StyleSheet.create({
  top10: { flexDirection: 'row', alignItems: 'center', flexWrap: 'wrap', gap: 6 },
  hero: { flexDirection: 'row', gap: 16, padding: 16, alignItems: 'flex-end', overflow: 'hidden' },
  heroWide: { padding: 24, gap: 24 },
  heroText: { flex: 1, gap: 6 },
  logoBox: { width: '100%', height: 64 },
  metaRow: { flexDirection: 'row', alignItems: 'center', flexWrap: 'wrap', gap: 8 },
  cert: { borderWidth: 1, borderRadius: 4, paddingHorizontal: 5, paddingVertical: 1 },
  // One row of equal boxes, however many sources there are (up to four).
  scores: { flexDirection: 'row', gap: 8 },
  score: { flex: 1, minWidth: 0, paddingVertical: 12, paddingHorizontal: 12, gap: 6, alignItems: 'flex-start' },
  providers: { flexDirection: 'row', flexWrap: 'wrap', gap: 14 },
  provider: { alignItems: 'center', gap: 4 },
  logo: { width: 48, height: 48, borderRadius: 12, borderWidth: StyleSheet.hairlineWidth },
  fact: { flexDirection: 'row', gap: 12, alignItems: 'baseline' },
});
