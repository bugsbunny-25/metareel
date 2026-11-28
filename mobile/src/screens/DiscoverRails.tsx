import { ActivityIndicator, FlatList, Pressable, StyleSheet, View } from 'react-native';

import { Icon } from '@/components/Icon';
import { Poster } from '@/components/Poster';
import { RatingLogo } from '@/components/RatingLogo';
import { Text } from '@/components/Text';
import { daysUntil, localDay } from '@/data/collections';
import { useElsewhere, useHiddenGems, useNewOnService, usePreferences } from '@/data/queries';
import { type LibraryState, libraryMark, providerName, useLibrary, useScope } from '@/data/scope';
import { useTheme } from '@/theme';

type Pick = (kind: 'movie' | 'tv', id: string, title?: string, poster?: string) => void;

/** One poster in a rail: what it is, where it came from, and a line under it. */
type Tile = { kind: 'movie' | 'tv'; id: string; title: string; poster?: string | null; line?: string; rating?: number | null };

const TILE = 112;
const PER_RAIL = 20;

/**
 * What Search shows before anything is typed: the web's Discover, lean. New
 * on the service your Top 10 follows (last 7 days), hidden gems streaming in
 * your country and what charts elsewhere but can't be streamed here. Titles
 * you've watched are left out of the first two, as on the web.
 */
export function DiscoverRails({ onPick }: { onPick: Pick }) {
  const prefs = usePreferences();
  const { country, provider } = useScope({ country: prefs.data?.country, provider: prefs.data?.provider });
  const library = useLibrary();
  const yours = library.data;
  const state = (t: Tile) => yours?.get(`${t.kind}:${t.id}`);
  const unseen = (t: Tile) => {
    const s = state(t);
    return !s || !((s.watch_count ?? 0) > 0 || s.status === 'completed');
  };

  const fresh = useNewOnService(country, provider);
  const gems = useHiddenGems(country);
  const elsewhere = useElsewhere(country);
  const today = localDay();

  const newTiles = dedupe(
    (fresh.data?.items ?? []).flatMap((i): Tile[] =>
      i.tmdb_id
        ? [
            {
              kind: i.kind,
              id: i.tmdb_id,
              title: i.title,
              poster: i.poster_url,
              line: [i.season_number ? `Season ${i.season_number}` : i.kind === 'tv' ? 'Series' : 'Film', i.date ? dayText(i.date, today) : '']
                .filter(Boolean)
                .join(' · '),
              rating: i.imdb_rating,
            },
          ]
        : [],
    ),
  ).filter(unseen);
  const gemTiles = (gems.data?.items ?? [])
    .flatMap((i): Tile[] =>
      i.tmdb_id && i.kind
        ? [{ kind: i.kind, id: i.tmdb_id, title: i.title ?? '', poster: i.poster_path, line: line(i.kind, i.year), rating: i.imdb_rating }]
        : [],
    )
    .filter(unseen);
  const elsewhereTiles = (elsewhere.data?.items ?? []).flatMap((i): Tile[] =>
    i.tmdb_id && i.kind
      ? [
          {
            kind: i.kind,
            id: i.tmdb_id,
            title: i.title ?? '',
            poster: i.poster_path,
            line: i.countries?.length ? `Top 10 in ${i.countries.length} ${i.countries.length === 1 ? 'country' : 'countries'}` : line(i.kind, i.year),
            rating: i.imdb_rating,
          },
        ]
      : [],
  );

  return (
    <View style={{ gap: 24, paddingTop: 12 }}>
      <Rail
        title={`New on ${providerName(provider)}`}
        hint={`Last 7 days in ${country}`}
        tiles={newTiles}
        query={fresh}
        state={state}
        onPick={onPick}
      />
      <Rail title="Hidden gems" hint={`Highly rated, streaming in ${country}, off the charts`} tiles={gemTiles} query={gems} state={state} onPick={onPick} />
      <Rail
        title="Popular elsewhere"
        hint={`Charting abroad, not streaming in ${country}`}
        tiles={elsewhereTiles}
        query={elsewhere}
        state={state}
        onPick={onPick}
      />
    </View>
  );
}

/** "Today", "Yesterday", else the weekday, for when a title arrived. */
function dayText(date: string, today: string): string {
  const ago = -daysUntil(date, today);
  if (ago <= 0) return 'Today';
  if (ago === 1) return 'Yesterday';
  return new Date(`${date.slice(0, 10)}T12:00:00`).toLocaleDateString(undefined, { weekday: 'short' });
}

const line = (kind: string, y?: number | null) => [kind === 'tv' ? 'Series' : 'Film', y ? String(y) : ''].filter(Boolean).join(' · ');

/** A show's new seasons come one per row; keep the first (newest) of each title. */
function dedupe(tiles: Tile[]): Tile[] {
  const seen = new Set<string>();
  return tiles.filter((t) => !seen.has(`${t.kind}:${t.id}`) && !!seen.add(`${t.kind}:${t.id}`));
}

/** A heading and a row of posters; hidden when the source has nothing (or fails). */
function Rail({
  title,
  hint,
  tiles,
  query,
  state,
  onPick,
}: {
  title: string;
  hint: string;
  tiles: Tile[];
  query: { isPending: boolean; isError: boolean };
  state: (t: Tile) => LibraryState | undefined;
  onPick: Pick;
}) {
  const { colors } = useTheme();
  if (!query.isPending && !tiles.length) return null;
  return (
    <View style={{ gap: 8 }}>
      <View style={styles.head}>
        <Text variant="heading">{title}</Text>
        <Text variant="caption">{hint}</Text>
      </View>
      {query.isPending ? (
        <ActivityIndicator color={colors.textMuted} style={{ alignSelf: 'flex-start', height: TILE * 1.5 }} />
      ) : (
        <FlatList
          horizontal
          data={tiles.slice(0, PER_RAIL)}
          keyExtractor={(t) => `${t.kind}-${t.id}`}
          showsHorizontalScrollIndicator={false}
          keyboardShouldPersistTaps="handled"
          contentContainerStyle={{ gap: 12, paddingBottom: 4 }}
          renderItem={({ item }) => <PosterCard tile={item} state={state(item)} onPick={onPick} />}
        />
      )}
    </View>
  );
}

function PosterCard({ tile: t, state, onPick }: { tile: Tile; state?: LibraryState; onPick: Pick }) {
  const { colors } = useTheme();
  const mark = libraryMark(state);
  return (
    <Pressable
      onPress={() => onPick(t.kind, t.id, t.title, t.poster ?? undefined)}
      accessibilityRole="button"
      accessibilityLabel={[t.title, t.line, t.rating ? `IMDb ${t.rating.toFixed(1)}` : '', state?.status === 'watchlist' ? 'on your watchlist' : '']
        .filter(Boolean)
        .join(', ')}
      style={({ pressed }) => [{ width: TILE, gap: 4, opacity: pressed ? 0.75 : 1 }]}
    >
      <View>
        <Poster path={t.poster} title={t.title} width={TILE} />
        {mark ? (
          <View style={[styles.mark, { backgroundColor: colors.accentYours }]}>
            <Icon name={mark} size={12} color={colors.bgCanvas} />
          </View>
        ) : null}
      </View>
      <Text variant="label" numberOfLines={1}>
        {t.title}
      </Text>
      {t.line ? (
        <Text variant="caption" num numberOfLines={1}>
          {t.line}
        </Text>
      ) : null}
      {t.rating ? (
        <View style={styles.meta}>
          <RatingLogo source="imdb" height={11} />
          <Text variant="caption" num>
            {t.rating.toFixed(1)}
          </Text>
        </View>
      ) : null}
    </Pressable>
  );
}

const styles = StyleSheet.create({
  head: { flexDirection: 'row', alignItems: 'baseline', gap: 10, flexWrap: 'wrap' },
  meta: { flexDirection: 'row', alignItems: 'center', gap: 4 },
  mark: { position: 'absolute', top: 6, right: 6, width: 22, height: 22, borderRadius: 11, alignItems: 'center', justifyContent: 'center' },
});
