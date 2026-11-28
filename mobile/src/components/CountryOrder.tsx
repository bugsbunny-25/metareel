import { useState } from 'react';
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, View } from 'react-native';

import { Chip } from '@/components/Chip';
import { Text } from '@/components/Text';
import { updatePreferences } from '@/data/preferences';
import { usePreferences } from '@/data/queries';
import { useChartReference } from '@/data/scope';
import { useTheme } from '@/theme';

/**
 * Your countries in order (the web's Settings "Country order"): a title page
 * charts its Top 10 run in the first of them it charted in. Each change is
 * saved to the account at once.
 */
export function CountryOrder() {
  const { colors } = useTheme();
  const prefs = usePreferences();
  const ref = useChartReference();
  const [adding, setAdding] = useState(false);
  const [busy, setBusy] = useState(false);
  const list = prefs.data?.countries ?? [];
  const name = (cc: string) => ref.data?.countries.find((c) => c.code === cc)?.name ?? cc;
  const save = async (next: string[]) => {
    setBusy(true);
    await updatePreferences({ countries: next }).finally(() => setBusy(false));
  };
  const move = (i: number, by: number) => {
    const next = [...list];
    const [c] = next.splice(i, 1);
    next.splice(i + by, 0, c);
    void save(next);
  };
  return (
    <View style={{ gap: 8 }}>
      <Text variant="muted">A title page shows its Top 10 run in the first of these countries it charted in. Empty uses the chart country you browse.</Text>
      {list.map((cc, i) => (
        <View key={cc} style={[styles.row, { backgroundColor: colors.bgElevated }]}>
          <Text variant="label" num style={{ width: 22, color: colors.accentYours }}>
            {i + 1}
          </Text>
          <Text style={{ flex: 1 }}>{name(cc)}</Text>
          <Mini label={`Move ${name(cc)} up`} text="↑" disabled={busy || i === 0} onPress={() => move(i, -1)} />
          <Mini label={`Move ${name(cc)} down`} text="↓" disabled={busy || i === list.length - 1} onPress={() => move(i, 1)} />
          <Mini label={`Remove ${name(cc)}`} text="×" disabled={busy} onPress={() => void save(list.filter((x) => x !== cc))} />
        </View>
      ))}
      <Chip label={adding ? 'Done' : 'Add a country'} selected={adding} onPress={() => setAdding((a) => !a)} />
      {adding ? (
        ref.data ? (
          <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 8 }}>
            {ref.data.countries
              .filter((c) => c.code && !list.includes(c.code))
              .map((c) => (
                <Chip key={c.code} label={c.name ?? c.code!} onPress={() => void save([...list, c.code!].slice(0, 20))} />
              ))}
          </ScrollView>
        ) : (
          <ActivityIndicator />
        )
      ) : null}
    </View>
  );
}

function Mini({ label, text, disabled, onPress }: { label: string; text: string; disabled?: boolean; onPress: () => void }) {
  return (
    <Pressable onPress={onPress} disabled={disabled} accessibilityRole="button" accessibilityLabel={label} hitSlop={4} style={[styles.mini, { opacity: disabled ? 0.3 : 1 }]}>
      <Text variant="label" style={{ fontSize: 17 }}>
        {text}
      </Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: 'row', alignItems: 'center', gap: 6, paddingLeft: 12, paddingRight: 4, minHeight: 44, borderRadius: 12, borderCurve: 'continuous' },
  mini: { width: 36, height: 36, alignItems: 'center', justifyContent: 'center' },
});
