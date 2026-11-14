import { useMemo, useState } from 'react';
import { View } from 'react-native';

import type { components } from '@/api/schema';
import { Badge, Countdown, MediaCard, PosterTile, TickButton } from '@/components/Media';
import { OutboxBanner } from '@/components/OutboxBanner';
import { Text } from '@/components/Text';
import { daysUntil, episodeBadge, type Group, hoursText, localDay, nextPerShow, tvToWatch, tvUpcoming } from '@/data/collections';
import { useShowQueue, useUpcoming } from '@/data/queries';
import { CollectionScreen } from '@/layout/CollectionScreen';
import { useTwoPane } from '@/layout/ListDetail';
import { markEpisode, openTitle } from '@/lib/actions';
import { episodeCode, runtimeText } from '@/lib/format';
import { usePersisted } from '@/lib/persisted';
import { useOutbox } from '@/offline';
import { ShowView } from '@/screens/ShowView';
import { useTheme } from '@/theme';

type Show = components['schemas']['ShowQueueItem'];
type Episode = components['schemas']['UpcomingEpisode'];
type Item = { type: 'show'; show: Show } | { type: 'episode'; ep: Episode };

const SEGMENTS = [
  { key: 'towatch' as const, label: 'To watch' },
  { key: 'upcoming' as const, label: 'Upcoming' },
];

/**
 * TV: the next episode of every show you're on (To watch), and what airs
 * next (Upcoming), as cards or a poster grid.
 */
export default function TV() {
  const [segment, setSegment] = usePersisted('tabs.tv.segment', 'towatch', ['towatch', 'upcoming']);
  const queue = useShowQueue();
  const upcoming = useUpcoming();
  const twoPane = useTwoPane();
  const [selected, setSelected] = useState<string | null>(null);
  const [view, setView] = usePersisted<'list' | 'grid'>('tabs.tv.view', 'list', ['list', 'grid']);
  const today = localDay();

  const open = (id: string, title?: string) => (twoPane ? setSelected(id) : openTitle({ kind: 'tv', id, title }));

  const shows = queue.data?.shows ?? [];
  const eps = upcoming.data?.episodes ?? [];
  const groups: Group<Item>[] = useMemo(() => {
    if (segment === 'towatch') return tvToWatch(shows).map((g) => ({ ...g, data: g.data.map((show) => ({ type: 'show' as const, show })) }));
    // The grid shows one poster per show; the list every episode by day.
    if (view === 'grid') {
      const next = nextPerShow(eps);
      return next.length ? [{ key: 'next', title: 'Next to air', data: next.map((ep) => ({ type: 'episode' as const, ep })) }] : [];
    }
    return tvUpcoming(eps, today).map((g) => ({ ...g, data: g.data.map((ep) => ({ type: 'episode' as const, ep })) }));
  }, [segment, view, shows, eps, today]);

  const summary = useMemo(() => {
    if (segment === 'towatch') {
      const on = shows.filter((s) => s.next_episode && ['watching', 'not_started', 'stalled', 'on_hold'].includes(s.stage ?? ''));
      if (!on.length) return null;
      const left = on.reduce((n, s) => n + (s.remaining_episodes ?? 0), 0);
      const mins = on.reduce((n, s) => n + (s.minutes_left ?? 0), 0);
      return `${on.length} ${on.length === 1 ? 'show' : 'shows'} · ${left} ${left === 1 ? 'episode' : 'episodes'} · ${hoursText(mins)} to catch up`;
    }
    if (!eps.length) return null;
    const week = eps.filter((e) => e.air_date && daysUntil(e.air_date, today) < 7).length;
    const showsCount = new Set(eps.map((e) => e.tmdb_id)).size;
    return `${week} ${week === 1 ? 'episode' : 'episodes'} this week · ${showsCount} ${showsCount === 1 ? 'show' : 'shows'} coming back`;
  }, [segment, shows, eps, today]);

  const q = segment === 'towatch' ? queue : upcoming;
  return (
    <CollectionScreen<Item, 'towatch' | 'upcoming'>
      title="TV"
      view={view}
      onView={setView}
      segments={SEGMENTS}
      segment={segment}
      onSegment={setSegment}
      groups={groups}
      itemKey={(i) => (i.type === 'show' ? `s${i.show.tmdb_id}` : `e${i.ep.tmdb_id}-${i.ep.season_number}-${i.ep.episode_number}`)}
      renderRow={(i) =>
        i.type === 'show' ? (
          <ShowCard show={i.show} selected={twoPane && selected === i.show.tmdb_id} onOpen={open} />
        ) : (
          <EpisodeCard ep={i.ep} today={today} selected={twoPane && selected === i.ep.tmdb_id} onOpen={open} />
        )
      }
      renderTile={(i, width) =>
        i.type === 'show' ? (
          <PosterTile
            width={width}
            poster={i.show.poster_path}
            title={i.show.title ?? ''}
            badge={episodeCode(i.show.next_episode?.season_number, i.show.next_episode?.episode_number)}
            percent={i.show.percent ?? 0}
            selected={twoPane && selected === i.show.tmdb_id}
            onPress={() => open(i.show.tmdb_id ?? '', i.show.title)}
          />
        ) : (
          <PosterTile
            width={width}
            poster={i.ep.poster_path}
            title={i.ep.show_title ?? ''}
            badge={countdownBadge(daysUntil(i.ep.air_date ?? today, today))}
            badgeSide="right"
            caption={episodeCode(i.ep.season_number, i.ep.episode_number)}
            selected={twoPane && selected === i.ep.tmdb_id}
            onPress={() => open(i.ep.tmdb_id ?? '', i.ep.show_title)}
          />
        )
      }
      summary={summary}
      loading={q.isPending}
      error={q.isError}
      empty={
        segment === 'towatch'
          ? 'Nothing to catch up on. Open a show and tick an episode to start.'
          : 'No episodes of your shows are scheduled in the next six months.'
      }
      refreshing={q.isRefetching}
      onRefresh={() => void Promise.all([queue.refetch(), upcoming.refetch()])}
      banner={<OutboxBanner kinds={['episodes']} />}
      detail={selected ? <ShowView key={selected} id={selected} embedded /> : null}
      placeholder="Pick a show to see it here."
    />
  );
}

const countdownBadge = (days: number) => (days <= 0 ? 'Today' : days === 1 ? 'Tomorrow' : `${days} days`);

function ShowCard({ show: s, selected, onOpen }: { show: Show; selected: boolean; onOpen: (id: string, title?: string) => void }) {
  const { colors } = useTheme();
  const ep = s.next_episode!;
  const pending = useOutbox().some(
    (q) =>
      q.kind === 'episodes' &&
      q.status === 'pending' &&
      q.meta?.show === s.tmdb_id &&
      q.meta?.season === ep.season_number &&
      q.meta?.episode === ep.episode_number,
  );
  const badge = episodeBadge(ep);
  const more = (s.remaining_episodes ?? 0) - 1;
  return (
    <MediaCard
      poster={s.poster_path}
      title={s.title ?? ''}
      selected={selected}
      onPress={() => onOpen(s.tmdb_id ?? '', s.title)}
      accessibilityLabel={`${s.title}, next ${episodeCode(ep.season_number, ep.episode_number)}`}
      right={
        <TickButton
          pending={pending}
          label={`Mark ${episodeCode(ep.season_number, ep.episode_number)} watched`}
          onPress={() => markEpisode(s.tmdb_id ?? '', s.title ?? '', ep.season_number!, ep.episode_number!)}
        />
      }
    >
      <Text variant="caption" numberOfLines={1} style={{ color: colors.accentYours, textTransform: 'uppercase', letterSpacing: 0.5 }}>
        {s.title}
      </Text>
      <View style={{ flexDirection: 'row', alignItems: 'baseline', gap: 6 }}>
        <Text variant="title" num>
          {episodeCode(ep.season_number, ep.episode_number)}
        </Text>
        {more > 0 ? (
          <Text variant="muted" num>
            +{more}
          </Text>
        ) : null}
      </View>
      <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
        {badge ? <Badge text={badge} /> : null}
        <Text variant="muted" numberOfLines={1} style={{ flexShrink: 1 }}>
          {[ep.name, runtimeText(ep.runtime)].filter(Boolean).join(' · ')}
        </Text>
      </View>
    </MediaCard>
  );
}

function EpisodeCard({ ep, today, selected, onOpen }: { ep: Episode; today: string; selected: boolean; onOpen: (id: string, title?: string) => void }) {
  const { colors } = useTheme();
  const badge = episodeBadge(ep);
  const days = daysUntil(ep.air_date ?? today, today);
  return (
    <MediaCard
      poster={ep.poster_path}
      title={ep.show_title ?? ''}
      selected={selected}
      onPress={() => onOpen(ep.tmdb_id ?? '', ep.show_title)}
      accessibilityLabel={`${ep.show_title}, ${episodeCode(ep.season_number, ep.episode_number)}, ${new Date(`${ep.air_date}T12:00:00`).toLocaleDateString()}`}
      right={<Countdown days={days} />}
    >
      <Text variant="caption" numberOfLines={1} style={{ color: colors.accentYours, textTransform: 'uppercase', letterSpacing: 0.5 }}>
        {ep.show_title}
      </Text>
      <Text variant="title" num>
        {episodeCode(ep.season_number, ep.episode_number)}
      </Text>
      <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
        {badge ? <Badge text={badge} /> : null}
        <Text variant="muted" numberOfLines={1} style={{ flexShrink: 1 }}>
          {[ep.name || 'TBA', runtimeText(ep.runtime)].filter(Boolean).join(' · ')}
        </Text>
      </View>
    </MediaCard>
  );
}
