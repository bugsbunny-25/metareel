import { useLocalSearchParams } from 'expo-router';

import { DiscView } from '@/screens/DiscView';

export default function DiscScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  return <DiscView id={Number(id)} />;
}
