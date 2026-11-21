import { useLocalSearchParams } from 'expo-router';

import { TitleView } from '@/screens/TitleView';

export default function TitleScreen() {
  const { kind, id } = useLocalSearchParams<{ kind: string; id: string }>();
  return <TitleView kind={kind} id={id} />;
}
