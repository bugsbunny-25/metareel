import { useLocalSearchParams } from 'expo-router';

import { DiscView } from '@/screens/DiscView';

/** A disc in your library: /disc/{copy}, a set inside a box set (?part=), or one disc (?disc=). */
export default function DiscScreen() {
  const { id, part, disc } = useLocalSearchParams<{ id: string; part?: string; disc?: string }>();
  return <DiscView id={Number(id)} part={part ? Number(part) : undefined} disc={disc ? Number(disc) : undefined} />;
}
