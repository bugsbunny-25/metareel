import { useMemo, useState } from 'react';

import type { components } from '@/api/schema';
import { Countdown, MediaCard, PosterTile, TickButton } from '@/components/Media';
import { OutboxBanner } from '@/components/OutboxBanner';
import { Text } from '@/components/Text';
import { daysUntil, type Group, hoursText, localDay, moviesSplit } from '@/data/collections';
import { useWatchlist } from '@/data/queries';
import { CollectionScreen } from '@/layout/CollectionScreen';
import { useTwoPane } from '@/layout/ListDetail';
import { openLog, openTitle } from '@/lib/actions';
import { runtimeText, year } from '@/lib/format';
import { usePersisted } from '@/lib/persisted';
import { TitleView } from '@/screens/TitleView';
import { useTheme } from '@/theme';

type Film = NonNullable<components['schemas']['TrackedTitleState']>;

const SEGMENTS = [
  { key: 'towatch' as const, label: 'To watch' },
  { key: 'upcoming' as const, label: 'Upcoming' },
];

/**
 * Movies: the films on your watchlist that are out (To watch, in your
 * watchlist order) and those still to come (Upcoming, by month with a
 * countdown). Ticking a film opens the log sheet, so the cinema, formats and
 * money of a watch are kept.
 */
export default function Movies() {
  const [segment, setSegment] = usePersisted('tabs.movies.segment', 'towatch', ['towatch', 'upcoming']);
  const [view, setView] = usePersisted<'list' | 'grid'>('tabs.movies.view', 'list', ['list', 'grid']);
  const list = useWatchlist();
  const twoPane = useTwoPane();
  const [selected, setSelected] = useState<string | null>(null);
  const today = localDay();
  const split = useMemo(() => moviesSplit(list.data?.titles ?? [], today), [list.data, today]);

  const groups: Group<Film>[] =
    segment === 'towatch' ? (split.toWatch.length ? [{ key: 'towatch', title: 'Out now', data: split.toWatch }] : []) : split.upcoming;

  const summary = useMemo(() => {
    if (segment === 'towatch') {
      if (!split.toWatch.length) return null;
      const mins = split.toWatch.reduce((n, f) => n + (f.runtime ?? 0), 0);
      return `${split.toWatch.length} ${split.toWatch.length === 1 ? 'film' : 'films'} · ${hoursText(mins)} of watching`;
    }
    const all = split.upcoming.flatMap((g) => g.data);
    if (!all.length) return null;
    const next = daysUntil(all[0].release_date!, today);
    return `${all.length} ${all.length === 1 ? 'film' : 'films'} coming · the next in ${next === 1 ? '1 day' : `${next} days`}`;
  }, [segment, split, today]);

  const open = (f: Film) => (twoPane ? setSelected(f.tmdb_id ?? null) : openTitle({ kind: 'movie', id: f.tmdb_id ?? '', title: f.title }));

  return (
    <CollectionScreen<Film, 'towatch' | 'upcoming'>
      title="Movies"
      view={view}
      onView={setView}
      segments={SEGMENTS}
      segment={segment}
      onSegment={setSegment}
      groups={groups}
      itemKey={(f) => f.tmdb_id ?? ''}
      renderRow={(f) => <FilmCard film={f} today={today} upcoming={segment === 'upcoming'} selected={twoPane && selected === f.tmdb_id} onOpen={open} />}
      renderTile={(f, width) => {
        const days = f.release_date ? daysUntil(f.release_date, today) : 0;
        return (
          <PosterTile
            width={width}
            poster={f.poster_path}
            title={f.title ?? ''}
            badge={segment === 'upcoming' ? (days === 1 ? 'Tomorrow' : `${days} days`) : null}
            badgeSide="right"
            caption={segment === 'upcoming' ? releaseText(f.release_date) : null}
            selected={twoPane && selected === f.tmdb_id}
            onPress={() => open(f)}
          />
        );
      }}
      summary={summary}
      loading={list.isPending}
      error={list.isError}
      empty={segment === 'towatch' ? 'No films waiting. Add some to your watchlist from a title page.' : 'No films on your watchlist are still to come.'}
      refreshing={list.isRefetching}
      onRefresh={() => void list.refetch()}
      banner={<OutboxBanner kinds={['watchlist', 'watch']} />}
      detail={selected ? <TitleView key={selected} kind="movie" id={selected} embedded /> : null}
      placeholder="Pick a film to see it here."
    />
  );
}

const releaseText = (date?: string) =>
  date ? new Date(`${date}T12:00:00`).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' }) : '';

function FilmCard({
  film: f,
  today,
  upcoming,
  selected,
  onOpen,
}: {
  film: Film;
  today: string;
  upcoming: boolean;
  selected: boolean;
  onOpen: (f: Film) => void;
}) {
  const { colors } = useTheme();
  return (
    <MediaCard
      poster={f.poster_path}
      title={f.title ?? ''}
      selected={selected}
      onPress={() => onOpen(f)}
      accessibilityLabel={f.title}
      right={
        upcoming ? (
          <Countdown days={daysUntil(f.release_date!, today)} />
        ) : (
          <TickButton label={`Log ${f.title}`} onPress={() => openLog({ kind: 'movie', id: f.tmdb_id ?? '', title: f.title, poster: f.poster_path })} />
        )
      }
    >
      <Text variant="heading" numberOfLines={2}>
        {f.title}
      </Text>
      {upcoming ? (
        <Text variant="label" style={{ color: colors.accentYours }}>
          {releaseText(f.release_date)}
        </Text>
      ) : (
        <Text variant="muted" num>
          {[year(f.release_date), runtimeText(f.runtime)].filter(Boolean).join(' · ')}
        </Text>
      )}
    </MediaCard>
  );
}
