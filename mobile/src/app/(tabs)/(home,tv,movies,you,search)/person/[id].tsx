import { useLocalSearchParams } from 'expo-router';

import { PersonView } from '@/screens/PersonView';

export default function PersonScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  return <PersonView id={Number(id)} />;
}
