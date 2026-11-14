import { useLocalSearchParams } from 'expo-router';

import { EpisodeView } from '@/screens/EpisodeView';

export default function EpisodeScreen() {
  const { show, season, episode } = useLocalSearchParams<{ show: string; season: string; episode: string }>();
  return <EpisodeView show={show} season={Number(season)} episode={Number(episode)} />;
}
