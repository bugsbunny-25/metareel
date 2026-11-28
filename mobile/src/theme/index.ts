import { useColorScheme } from 'react-native';

import { useThemePreference } from './preference';
import { tokens } from './tokens';

export type Scheme = 'dark' | 'light';
export type Colors = { [K in keyof typeof tokens.dark]: string };

/** Font family names as loaded in the root layout (@expo-google-fonts, and assets/fonts). */
export const fonts = {
  display: 'SourceSerif4_700Bold',
  displaySemi: 'SourceSerif4_600SemiBold',
  sans: 'SourceSans3_400Regular',
  sansSemi: 'SourceSans3_600SemiBold',
  sansBold: 'SourceSans3_700Bold',
  /** The chart rank numerals (Numeral.tsx). */
  numeral: 'BarlowCondensed_800ExtraBold',
  /**
   * Age ratings (CertificationBadge): Motion Picture Ratings (Daniel Lyons
   * Entertainment, all rights reserved), the font tides uses. It draws only
   * A-Z, a-z, 0-9, "-" and ".".
   */
  certification: 'MotionPictureRatings',
} as const;

/** Lining, tabular figures for numbers that line up (the web's `.num`). */
export const tabularNums = ['lining-nums', 'tabular-nums'] as ('lining-nums' | 'tabular-nums')[];

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
