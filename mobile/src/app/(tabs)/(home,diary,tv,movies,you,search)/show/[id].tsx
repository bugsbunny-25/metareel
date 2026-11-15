import { useLocalSearchParams } from 'expo-router';

import { ShowView } from '@/screens/ShowView';

export default function ShowScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  return <ShowView id={id} />;
}
