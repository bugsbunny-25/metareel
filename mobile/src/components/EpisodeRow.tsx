import { StyleSheet, View } from 'react-native';

import type { components } from '@/api/schema';

import { Text } from './Text';

type Episode = components['schemas']['UpNextEpisode'];

export function episodeCode(e: Pick<Episode, 'season_number' | 'episode_number'>): string {
  const pad = (n?: number) => String(n ?? 0).padStart(2, '0');
  return `S${pad(e.season_number)} E${pad(e.episode_number)}`;
}

export function EpisodeRow({ episode }: { episode: Episode }) {
  return (
    <View style={styles.row} accessible accessibilityLabel={`${episode.show_title}, ${episodeCode(episode)}, ${episode.name ?? ''}`}>
      <Text variant="label" numberOfLines={1}>
        {episode.show_title}
      </Text>
      <Text variant="muted" numberOfLines={1}>
        <Text variant="muted" num>
          {episodeCode(episode)}
        </Text>
        {episode.name ? ` · ${episode.name}` : ''}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({ row: { gap: 2, paddingVertical: 6 } });
