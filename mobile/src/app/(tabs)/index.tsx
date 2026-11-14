import { ActivityIndicator } from 'react-native';

import { ConnectionNote } from '@/components/ConnectionNote';
import { EpisodeRow } from '@/components/EpisodeRow';
import { Panel, Screen } from '@/components/Screen';
import { Text } from '@/components/Text';
import { useUpNext } from '@/data/queries';
import { useTheme } from '@/theme';

export default function Tonight() {
  const upNext = useUpNext(5);
  const { colors } = useTheme();
  return (
    <Screen title="Tonight" refreshing={upNext.isRefetching} onRefresh={() => void upNext.refetch()}>
      <ConnectionNote />
      <Panel title="Up next">
        {upNext.isPending ? (
          <ActivityIndicator color={colors.textMuted} />
        ) : upNext.isError && !upNext.data ? (
          <Text variant="muted">Couldn&apos;t load Up next. Pull to try again.</Text>
        ) : upNext.data?.episodes?.length ? (
          upNext.data.episodes.map((e) => <EpisodeRow key={`${e.tmdb_id}-${e.season_number}-${e.episode_number}`} episode={e} />)
        ) : (
          <Text variant="muted">Nothing in progress.</Text>
        )}
      </Panel>
    </Screen>
  );
}
