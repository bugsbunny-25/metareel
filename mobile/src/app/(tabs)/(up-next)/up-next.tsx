import { ActivityIndicator, Pressable, SectionList, StyleSheet, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import type { components } from '@/api/schema';
import { Icon } from '@/components/Icon';
import { OutboxBanner } from '@/components/OutboxBanner';
import { Text } from '@/components/Text';
import { TitleRow } from '@/components/TitleRow';
import { useShowQueue } from '@/data/queries';
import { markEpisode, openTitle } from '@/lib/actions';
import { episodeCode } from '@/lib/format';
import { useOutbox } from '@/offline';
import { useTheme } from '@/theme';

type Item = components['schemas']['ShowQueueItem'];

const STAGES: { stage: NonNullable<Item['stage']>; title: string }[] = [
  { stage: 'watching', title: 'Continue watching' },
  { stage: 'stalled', title: "Haven't watched in a while" },
  { stage: 'not_started', title: 'Not started' },
  { stage: 'on_hold', title: 'On hold' },
];

/** Shows by stage, each with its next episode and a tick to mark it watched. */
export default function UpNext() {
  const { colors, gutter } = useTheme();
  const insets = useSafeAreaInsets();
  const queue = useShowQueue();
  const queued = useOutbox().filter((i) => i.kind === 'episodes' && i.status === 'pending');
  const sections = STAGES.map((s) => ({
    title: s.title,
    data: (queue.data?.shows ?? []).filter((x) => x.stage === s.stage && x.next_episode),
  })).filter((s) => s.data.length);

  return (
    <SectionList
      style={{ backgroundColor: colors.bgCanvas }}
      contentContainerStyle={{ paddingTop: insets.top + 12, paddingHorizontal: gutter.compact, paddingBottom: 32, width: '100%', maxWidth: 760, alignSelf: 'center' }}
      sections={sections}
      keyExtractor={(x) => x.tmdb_id ?? ''}
      stickySectionHeadersEnabled={false}
      onRefresh={() => void queue.refetch()}
      refreshing={queue.isRefetching}
      ListHeaderComponent={
        <View style={{ gap: 12, marginBottom: 8 }}>
          <Text variant="display" accessibilityRole="header">
            Up next
          </Text>
          <OutboxBanner kinds={['episodes']} />
        </View>
      }
      renderSectionHeader={({ section }) => (
        <Text variant="caption" style={{ marginTop: 14, marginBottom: 4 }}>
          {section.title}
        </Text>
      )}
      renderItem={({ item: x }) => {
        const ep = x.next_episode!;
        const pending = queued.some((q) => q.meta?.show === x.tmdb_id && q.meta?.season === ep.season_number && q.meta?.episode === ep.episode_number);
        return (
          <TitleRow
            poster={x.poster_path}
            title={x.title ?? ''}
            subtitle={
              <Text variant="muted" numberOfLines={1}>
                <Text variant="muted" num>
                  {episodeCode(ep.season_number, ep.episode_number)}
                </Text>
                {ep.name ? ` · ${ep.name}` : ''}
                {x.remaining_episodes ? ` · ${x.remaining_episodes} left` : ''}
              </Text>
            }
            onPress={() => openTitle({ kind: 'tv', id: x.tmdb_id ?? '' })}
            right={
              <Pressable
                disabled={pending}
                onPress={() => markEpisode(x.tmdb_id ?? '', x.title ?? '', ep.season_number!, ep.episode_number!)}
                accessibilityRole="button"
                accessibilityLabel={`Mark ${episodeCode(ep.season_number, ep.episode_number)} watched`}
                hitSlop={8}
                style={styles.tick}
              >
                <Icon name="check-circle" size={30} color={pending ? colors.accentYours : colors.textMuted} />
              </Pressable>
            }
          />
        );
      }}
      ListEmptyComponent={
        queue.isPending ? (
          <ActivityIndicator color={colors.textMuted} />
        ) : (
          <Text variant="muted">No shows in progress. Open a show and tick an episode to start.</Text>
        )
      }
    />
  );
}

const styles = StyleSheet.create({ tick: { width: 44, height: 44, alignItems: 'center', justifyContent: 'center' } });
