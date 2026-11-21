import { Image } from 'expo-image';
import { useState } from 'react';
import { Pressable, ScrollView, StyleSheet, View } from 'react-native';

import { Panel } from '@/components/Screen';
import { Text } from '@/components/Text';
import { useTitleCredits } from '@/data/queries';
import { openPerson } from '@/lib/actions';
import { tmdbImage } from '@/lib/format';
import { useTheme } from '@/theme';

// A title's cast and crew, as on the web's title page: faces in a row (each
// opens the person), then the crew as the lines a credits block reads as.

const CAST_SHOWN = 15;

// Crew jobs in the order a credits block names them, with their line.
const CREW_LINES: [string, string[]][] = [
  ['Created by', ['Creator']],
  ['Directed by', ['Director']],
  ['Written by', ['Screenplay', 'Writer', 'Teleplay', 'Story']],
  ['Based on', ['Novel', 'Characters']],
  ['Music', ['Original Music Composer']],
  ['Cinematography', ['Director of Photography']],
];

/** A round face, or initials when TMDB has no photo. */
export function Face({ path, name, size }: { path?: string | null; name: string; size: number }) {
  const { colors } = useTheme();
  const uri = tmdbImage(path, size > 90 ? 'w342' : 'w185');
  return (
    <View style={[styles.face, { width: size, height: size, borderRadius: size / 2, backgroundColor: colors.bgElevated, borderColor: colors.posterHairline }]}>
      {uri ? (
        <Image source={{ uri }} style={StyleSheet.absoluteFill} contentFit="cover" contentPosition="top" transition={150} accessible={false} />
      ) : (
        <Text variant="label" style={{ color: colors.textMuted }}>
          {name
            .split(/\s+/)
            .slice(0, 2)
            .map((w) => w[0])
            .join('')}
        </Text>
      )}
    </View>
  );
}

export function CastAndCrew({ kind, id }: { kind: 'movie' | 'tv'; id: string }) {
  const { colors } = useTheme();
  const credits = useTitleCredits(kind, id);
  const [all, setAll] = useState(false);
  const cast = credits.data?.cast ?? [];
  const crew = credits.data?.crew ?? [];
  const lines = CREW_LINES.map(([label, jobs]) => {
    const seen = new Set<number>();
    const people = crew.filter((c) => c.job && jobs.includes(c.job) && c.person_id && !seen.has(c.person_id) && seen.add(c.person_id));
    return [label, people] as const;
  }).filter(([, people]) => people.length);
  if (!cast.length && !lines.length) return null;
  const shown = all ? cast : cast.slice(0, CAST_SHOWN);
  return (
    <Panel title="Cast & crew">
      {cast.length ? (
        <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 14 }}>
          {shown.map((c) => (
            <Pressable
              key={`${c.person_id}-${c.character}`}
              onPress={() => c.person_id && openPerson(c.person_id)}
              accessibilityRole="button"
              accessibilityLabel={[c.name, c.character ? `as ${c.character}` : '', c.episodes ? `${c.episodes} episodes` : ''].filter(Boolean).join(', ')}
              style={({ pressed }) => [styles.member, { opacity: pressed ? 0.7 : 1 }]}
            >
              <Face path={c.profile_path} name={c.name ?? ''} size={72} />
              <Text variant="label" numberOfLines={2} style={styles.center}>
                {c.name}
              </Text>
              {c.character ? (
                <Text variant="caption" numberOfLines={2} style={styles.center}>
                  {c.character}
                </Text>
              ) : null}
              {kind === 'tv' && c.episodes ? (
                <Text variant="caption" num style={styles.center}>
                  {c.episodes} {c.episodes === 1 ? 'episode' : 'episodes'}
                </Text>
              ) : null}
            </Pressable>
          ))}
          {!all && cast.length > CAST_SHOWN ? (
            <Pressable onPress={() => setAll(true)} accessibilityRole="button" style={[styles.member, styles.more]}>
              <View style={[styles.moreDot, { backgroundColor: colors.bgElevated }]}>
                <Text variant="label" num>
                  +{cast.length - CAST_SHOWN}
                </Text>
              </View>
              <Text variant="caption" style={styles.center}>
                All {cast.length} cast
              </Text>
            </Pressable>
          ) : null}
        </ScrollView>
      ) : null}
      {lines.length ? (
        <View style={{ gap: 6 }}>
          {lines.map(([label, people]) => (
            <View key={label} style={styles.line}>
              <Text variant="caption" style={{ width: 110 }}>
                {label}
              </Text>
              <View style={styles.names}>
                {people.map((p, i) => (
                  <Pressable key={p.person_id} onPress={() => openPerson(p.person_id!)} accessibilityRole="link" hitSlop={6}>
                    <Text variant="body" style={{ color: colors.textPrimary, textDecorationLine: 'underline', textDecorationColor: colors.borderSubtle }}>
                      {p.name}
                      {i < people.length - 1 ? ',' : ''}
                    </Text>
                  </Pressable>
                ))}
              </View>
            </View>
          ))}
        </View>
      ) : null}
      {credits.data?.source === 'tmdb' ? <Text variant="caption">From TMDB</Text> : null}
    </Panel>
  );
}

const styles = StyleSheet.create({
  face: { overflow: 'hidden', alignItems: 'center', justifyContent: 'center', borderWidth: StyleSheet.hairlineWidth },
  member: { width: 84, alignItems: 'center', gap: 4 },
  center: { textAlign: 'center' },
  more: { justifyContent: 'flex-start' },
  moreDot: { width: 72, height: 72, borderRadius: 36, alignItems: 'center', justifyContent: 'center' },
  line: { flexDirection: 'row', gap: 12, alignItems: 'baseline' },
  names: { flex: 1, flexDirection: 'row', flexWrap: 'wrap', columnGap: 6, rowGap: 2 },
});
