import { Pressable, StyleSheet, View } from 'react-native';

import { flushOutbox, outbox, type OutboxKind, useOutbox } from '@/offline';
import { useTheme } from '@/theme';

import { Text } from './Text';

/**
 * What hasn't reached the server yet: a quiet line while writes are queued,
 * and each one the server refused, with Retry and Discard.
 */
export function OutboxBanner({ kinds }: { kinds?: OutboxKind[] }) {
  const { colors, radius } = useTheme();
  const items = useOutbox().filter((i) => !kinds || kinds.includes(i.kind));
  if (!items.length) return null;
  const pending = items.filter((i) => i.status === 'pending');
  const failed = items.filter((i) => i.status === 'failed');
  return (
    <View style={{ gap: 8 }}>
      {pending.length ? (
        <Pressable
          onPress={() => void flushOutbox()}
          accessibilityRole="button"
          accessibilityHint="Tries to send them now"
          style={[styles.box, { backgroundColor: colors.bgSurface, borderRadius: radius.panel }]}
        >
          <Text variant="label">
            {pending.length === 1 ? '1 change' : `${pending.length} changes`} waiting to sync
          </Text>
          <Text variant="muted" numberOfLines={2}>
            {pending.map((i) => i.label).join(', ')}
            {pending[0].lastError ? ` · ${pending[0].lastError}` : ''}
          </Text>
        </Pressable>
      ) : null}
      {failed.map((i) => (
        <View key={i.id} style={[styles.box, { backgroundColor: colors.bgSurface, borderRadius: radius.panel }]}>
          <Text variant="label" style={{ color: colors.deltaDown }}>
            Couldn&apos;t sync {i.label}
          </Text>
          {i.lastError ? <Text variant="muted">{i.lastError}</Text> : null}
          <View style={styles.actions}>
            <Pressable onPress={() => void outbox.retry(i.id)} accessibilityRole="button" style={styles.action}>
              <Text variant="label">Retry</Text>
            </Pressable>
            <Pressable onPress={() => outbox.discard(i.id)} accessibilityRole="button" style={styles.action}>
              <Text variant="label" style={{ color: colors.deltaDown }}>
                Discard
              </Text>
            </Pressable>
          </View>
        </View>
      ))}
    </View>
  );
}

const styles = StyleSheet.create({
  box: { padding: 14, gap: 4 },
  actions: { flexDirection: 'row', gap: 20, marginTop: 4 },
  action: { minHeight: 32, justifyContent: 'center' },
});
