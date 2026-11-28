import { Image, type ImageProps } from 'expo-image';
import { StyleSheet, View } from 'react-native';

import { currentSession, endpoint } from '@/auth/session';
import { Text } from '@/components/Text';
import { tmdbImage } from '@/lib/format';
import { fonts, useTheme } from '@/theme';

import { caseOf, coverOf, type DiscRelease, imageOf, isBoxSet } from './vocab';

/** A disc picture: served by metareel, so the session token goes with it. Cached for good (the URL is its hash). */
export function DiscImage({ url, ...rest }: Omit<ImageProps, 'source'> & { url: string }) {
  const base = endpoint.base();
  const token = currentSession()?.token;
  return (
    <Image
      source={{
        uri: `${base}${url}`,
        headers: token ? { Authorization: `Bearer ${token}` } : undefined,
      }}
      cachePolicy="disk"
      transition={120}
      accessible={false}
      {...rest}
    />
  );
}

/**
 * A release as its case: the front art (your photo or the lookup's, else the
 * film's TMDB poster) under the format's band, in the case's proportions.
 * Steelbooks get a metal edge and no band; a slipcover is a small tag.
 */
export function DiscCase({ release, width }: { release: DiscRelease; width: number }) {
  const { colors } = useTheme();
  const look = caseOf(release.media_format);
  const cover = coverOf(release);
  const steel = release.packaging === 'steelbook';
  const poster = cover.poster ? tmdbImage(cover.poster, width > 120 ? 'w342' : 'w185') : null;
  return (
    <View
      style={[
        styles.case,
        {
          width,
          height: Math.round(width / look.ratio),
          backgroundColor: steel ? '#9a9aa2' : look.band,
          padding: steel ? 3 : 0,
        },
      ]}
    >
      {look.mark && !steel ? (
        <Text style={[styles.band, { color: look.text, fontSize: width < 90 ? 7 : 9 }]} numberOfLines={1}>
          {look.mark}
        </Text>
      ) : null}
      <View style={[styles.art, { backgroundColor: colors.bgElevated, borderRadius: steel ? 4 : 0, borderCurve: 'continuous' }]}>
        {cover.image ? (
          <DiscImage url={width > 140 ? cover.image.url : cover.image.thumb_url} style={StyleSheet.absoluteFill} contentFit="cover" />
        ) : poster ? (
          <Image source={{ uri: poster }} style={StyleSheet.absoluteFill} contentFit="cover" accessible={false} />
        ) : (
          <Text variant="caption" style={styles.ph} numberOfLines={4}>
            {release.title}
          </Text>
        )}
      </View>
      {release.has_slipcover ? <Text style={styles.tag}>SLIP</Text> : null}
    </View>
  );
}

/**
 * A release's spine, for the shelf: your spine photo when there is one,
 * otherwise one made from the cover (blurred, so it reads as the case's
 * colour) with the format's mark at the top and the title running down it.
 * Thickness follows the number of discs; a box set is a wide block.
 */
export function DiscSpine({ release, height, lifted }: { release: DiscRelease; height: number; lifted?: boolean }) {
  const look = caseOf(release.media_format);
  const spine = imageOf(release, 'spine');
  const cover = coverOf(release);
  const box = isBoxSet(release);
  const h = Math.round(height * (look.ratio < 0.75 ? 1.1 : 1));
  const w = Math.round(
    height * (box ? 0.2 + Math.min(release.parts.length || release.disc_count, 8) * 0.025 : 0.09 + Math.min(Math.max(release.disc_count - 1, 0), 3) * 0.02),
  );
  const poster = cover.poster ? tmdbImage(cover.poster, 'w185') : null;
  const band = 16;
  return (
    <View
      style={[
        styles.spine,
        {
          width: w,
          height: h,
          backgroundColor: look.band,
          transform: [{ translateY: lifted ? -14 : 0 }],
        },
      ]}
    >
      {spine ? (
        <DiscImage url={spine.thumb_url} style={StyleSheet.absoluteFill} contentFit="cover" />
      ) : (
        <>
          {cover.image ? (
            <DiscImage url={cover.image.thumb_url} style={StyleSheet.absoluteFill} contentFit="cover" blurRadius={14} />
          ) : poster ? (
            <Image source={{ uri: poster }} style={StyleSheet.absoluteFill} contentFit="cover" blurRadius={14} accessible={false} />
          ) : null}
          <View style={[StyleSheet.absoluteFill, { backgroundColor: 'rgba(0,0,0,0.45)' }]} />
          <View
            style={{
              height: band,
              backgroundColor: look.band,
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            <Text
              style={{
                color: look.text,
                fontSize: 7,
                fontWeight: '800',
                letterSpacing: 0.5,
              }}
              numberOfLines={1}
            >
              {look.spine}
            </Text>
          </View>
          {/* The title runs down the spine: a box as long as the spine, turned a quarter. */}
          <View
            style={{
              position: 'absolute',
              top: band + (h - band) / 2 - w / 2,
              left: w / 2 - (h - band - 12) / 2,
              width: h - band - 12,
              height: w,
              justifyContent: 'center',
              transform: [{ rotate: '90deg' }],
            }}
          >
            <Text numberOfLines={1} style={[styles.spineTitle, { fontSize: box ? 14 : Math.min(13, w * 0.55) }]}>
              {release.title}
            </Text>
          </View>
        </>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  case: {
    overflow: 'hidden',
    borderTopLeftRadius: 5,
    borderBottomLeftRadius: 5,
    borderTopRightRadius: 8,
    borderBottomRightRadius: 8,
  },
  band: {
    textAlign: 'center',
    fontWeight: '800',
    letterSpacing: 1.2,
    paddingVertical: 3,
  },
  art: { flex: 1, overflow: 'hidden' },
  ph: {
    padding: 8,
    textAlign: 'center',
    marginTop: 'auto',
    marginBottom: 'auto',
  },
  tag: {
    position: 'absolute',
    right: 5,
    bottom: 5,
    paddingHorizontal: 5,
    paddingVertical: 2,
    borderRadius: 4,
    borderCurve: 'continuous',
    overflow: 'hidden',
    backgroundColor: 'rgba(0,0,0,0.72)',
    color: '#fff',
    fontSize: 9,
    fontWeight: '700',
  },
  spine: { overflow: 'hidden', borderRadius: 3, borderCurve: 'continuous' },
  spineTitle: {
    color: '#fff',
    fontFamily: fonts.display,
    textShadowColor: 'rgba(0,0,0,0.6)',
    textShadowRadius: 2,
  },
});
