import { Stack } from 'expo-router';
import { ActivityIndicator, ScrollView, StyleSheet, View } from 'react-native';

import { Panel } from '@/components/Screen';
import { Text } from '@/components/Text';
import { useSavings } from '@/data/queries';
import { formatMoney } from '@/lib/format';
import { useTheme } from '@/theme';

/**
 * Your cinema subscriptions and whether they pay off: this period's covered
 * films against the break-even count, and the all-time balance. Plans are
 * added and edited on the web.
 */
export default function Subscriptions() {
  const { colors, gutter } = useTheme();
  const savings = useSavings();
  const subs = savings.data?.subscriptions ?? [];
  return (
    <ScrollView style={{ backgroundColor: colors.bgCanvas }} contentContainerStyle={{ padding: gutter.compact, gap: 16 }}>
      <Stack.Screen options={{ headerTitle: 'Subscriptions' }} />
      {savings.isPending ? <ActivityIndicator color={colors.textMuted} /> : null}
      {!savings.isPending && !subs.length ? (
        <Panel>
          <Text variant="muted">No cinema subscriptions yet. Add one on the web (Settings → Cinema subscriptions).</Text>
        </Panel>
      ) : null}
      {subs.map((s) => {
        const month = s.months?.[s.months.length - 1];
        const need = s.break_even_watches ?? null;
        const done = month?.watches ?? 0;
        const saved = s.saved_minor ?? 0;
        return (
          <Panel key={s.id} title={s.plan}>
            <Text variant="muted">
              {formatMoney(s.price_minor, s.currency)} a {s.period}
              {s.active ? '' : ' · ended'}
            </Text>
            {s.active && need ? (
              <View style={{ gap: 6 }}>
                <Text>
                  <Text variant="label" num>
                    {done} of {need}
                  </Text>{' '}
                  films this month to break even
                </Text>
                <Meter value={done} max={need} />
              </View>
            ) : null}
            <Text>
              {saved >= 0 ? 'Saved ' : 'Cost '}
              <Text variant="label" num style={{ color: saved >= 0 ? colors.deltaUp : colors.deltaDown }}>
                {formatMoney(Math.abs(saved), s.currency)}
              </Text>{' '}
              over <Text num>{s.watches ?? 0}</Text> covered {s.watches === 1 ? 'film' : 'films'}
            </Text>
            {s.estimated ? <Text variant="caption">{s.estimated} valued at your usual ticket price</Text> : null}
          </Panel>
        );
      })}
    </ScrollView>
  );
}

function Meter({ value, max }: { value: number; max: number }) {
  const { colors } = useTheme();
  const pct = Math.min(100, (value / Math.max(1, max)) * 100);
  return (
    <View style={[styles.meter, { backgroundColor: colors.bgElevated }]} accessible accessibilityLabel={`${value} of ${max}`}>
      <View style={{ width: `${pct}%`, height: '100%', borderRadius: 4, backgroundColor: value >= max ? colors.deltaUp : colors.accentYours }} />
    </View>
  );
}

const styles = StyleSheet.create({ meter: { height: 8, borderRadius: 4, overflow: 'hidden' } });
