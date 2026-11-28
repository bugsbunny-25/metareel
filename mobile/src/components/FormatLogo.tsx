import { Image } from 'expo-image';
import { useState } from 'react';
import { StyleSheet, View } from 'react-native';

import { currentSession, endpoint } from '@/auth/session';

/**
 * A premium format's or disc's logo on a dark plate (logos are often white,
 * and the plate keeps them legible in the light theme too), like the web's
 * FormatBadge. The logo route needs the session, so the token goes with the
 * request; "mono" draws a dark logo white. Nothing when it has no logo or the
 * image fails, so the label alone stands.
 */
export function FormatLogo({ url, mono, height = 14, maxWidth = 64 }: { url?: string | null; mono?: boolean; height?: number; maxWidth?: number }) {
  const [broken, setBroken] = useState(false);
  const base = endpoint.base();
  const token = currentSession()?.token;
  if (!url || !base || broken) return null;
  return (
    <View style={[styles.plate, { height: height + 8 }]} accessible={false}>
      <Image
        source={{ uri: `${base}${url}`, headers: token ? { Authorization: `Bearer ${token}` } : undefined }}
        style={{ height, width: maxWidth }}
        tintColor={mono ? '#ffffff' : undefined}
        contentFit="contain"
        contentPosition="center"
        cachePolicy="disk"
        onError={() => setBroken(true)}
      />
    </View>
  );
}

/** Whether a format has a logo the plate can show. */
export const hasLogo = (f: { logo_url?: string | null }) => !!f.logo_url;

const styles = StyleSheet.create({
  plate: { paddingHorizontal: 6, borderRadius: 8, borderCurve: 'continuous', backgroundColor: '#161618', alignItems: 'center', justifyContent: 'center' },
});
