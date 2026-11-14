import { ActivityIndicator } from 'react-native';

import { EpisodeRow } from '@/components/EpisodeRow';
import { Panel, Screen } from '@/components/Screen';
import { Text } from '@/components/Text';
import { useUpNext } from '@/data/queries';
import { useTheme } from '@/theme';

export default function UpNext() {
  const upNext = useUpNext(50);
  const { colors } = useTheme();
  return (
    <Screen title="Up next" refreshing={upNext.isRefetching} onRefresh={() => void upNext.refetch()}>
      <Panel title="Continue watching">
        {upNext.isPending ? (
          <ActivityIndicator color={colors.textMuted} />
        ) : upNext.data?.episodes?.length ? (
          upNext.data.episodes.map((e) => <EpisodeRow key={`${e.tmdb_id}-${e.season_number}-${e.episode_number}`} episode={e} />)
        ) : (
          <Text variant="muted">Nothing in progress.</Text>
        )}
      </Panel>
    </Screen>
  );
}
