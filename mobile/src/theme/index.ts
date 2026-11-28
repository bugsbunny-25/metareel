import { useColorScheme } from 'react-native';

import { useThemePreference } from './preference';
import { tokens } from './tokens';

export type Scheme = 'dark' | 'light';
export type Colors = { [K in keyof typeof tokens.dark]: string };

// Embedded by the expo-font config plugin (app.config.ts). Android names a
// family after its file; iOS after the font's PostScript name.
const ios = process.env.EXPO_OS === 'ios';

/** Font family names, as each platform knows the embedded fonts. */
export const fonts = {
  display: ios ? 'SourceSerif4-Bold' : 'SourceSerif4_700Bold',
  displaySemi: ios ? 'SourceSerif4-SemiBold' : 'SourceSerif4_600SemiBold',
  sans: ios ? 'SourceSans3-Regular' : 'SourceSans3_400Regular',
  sansSemi: ios ? 'SourceSans3-SemiBold' : 'SourceSans3_600SemiBold',
  sansBold: ios ? 'SourceSans3-Bold' : 'SourceSans3_700Bold',
  /** The chart rank numerals (Numeral.tsx). */
  numeral: ios ? 'BarlowCondensed-ExtraBold' : 'BarlowCondensed_800ExtraBold',
  /**
   * Age ratings (CertificationBadge): Motion Picture Ratings (Daniel Lyons
   * Entertainment, all rights reserved), the font tides uses. It draws only
   * A-Z, a-z, 0-9, "-" and ".".
   */
  certification: 'MotionPictureRatings',
} as const;

/** Lining, tabular figures for numbers that line up (the web's `.num`). */
export const tabularNums = ['lining-nums', 'tabular-nums'] as ('lining-nums' | 'tabular-nums')[];

/** Text over artwork (stills, scrims, poster badges): the dark theme's primary text, in either theme. */
export const onArtwork = tokens.dark.textPrimary;

/** The white behind retail product photos, which come shot on white. */
export const photoMatte = tokens.light.bgSurface;

/**
 * Dark first: "system" means light only when the OS asks for light, so a
 * device with no preference gets the Screening Room's dark canvas.
 */
export function resolveScheme(pref: string, system: string | null | undefined): Scheme {
  if (pref === 'dark' || pref === 'light') return pref;
  return system === 'light' ? 'light' : 'dark';
}

export function useTheme() {
  const pref = useThemePreference();
  const system = useColorScheme();
  const scheme = resolveScheme(pref, system);
  return {
    scheme,
    colors: tokens[scheme] as Colors,
    radius: tokens.radius,
    gutter: tokens.gutter,
    fonts,
  };
}

export type Theme = ReturnType<typeof useTheme>;
