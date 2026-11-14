import { useLocalSearchParams } from 'expo-router';

import { WatchView } from '@/screens/WatchView';

export default function WatchScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  return <WatchView id={id} />;
}
