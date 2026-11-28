import { Text as RNText, StyleSheet, View } from 'react-native';

import { fonts, useTheme } from '@/theme';

import { Text } from './Text';

// Clear space, border and corner radius as a share of the text size, as tides'
// CertificationBadge (measured from an MPA rating box). The border is as heavy
// as the font's strokes, so box and letters read as one mark.
const PADDING_X = 0.35;
const PADDING_Y = 0.2;
const CORNER = 0.25;
const STROKE = 0.1;

/**
 * Characters the certification font draws: its letters, digits, '-' and '.',
 * plus the '/' and '+' drawn into it to match (tools/cert-font-glyphs.py) and
 * the space. The rest stay in the surrounding font.
 */
const IN_FONT = /[A-Za-z0-9.\-/+ ]/;

/** A rating split into runs: those the certification font draws, and the rest. */
export function certificationRuns(rating: string): { text: string; cert: boolean }[] {
  const runs: { text: string; cert: boolean }[] = [];
  for (const ch of rating) {
    const cert = IN_FONT.test(ch);
    const last = runs[runs.length - 1];
    if (last && last.cert === cert) last.text += ch;
    else runs.push({ text: ch, cert });
  }
  return runs;
}

// The line is trimmed to the font's ascent + descent (1716 + 418 of 2048 units),
// as tides' LineHeightStyle.Trim.Both, so the vertical space is the padding alone.
const LINE = (1716 + 418) / 2048;

/**
 * An age rating ("PG-13", "U/A 16+") in the certification font inside a
 * rounded border of the same weight, like the rating box on a film poster:
 * clear space on every side and slightly rounded corners, not a pill. Border,
 * corners and clear space are tides' CertificationBadge shares of the text
 * size, so the box keeps its proportions at whatever size a screen uses (the
 * size of the text beside it). The font is much heavier than the text around
 * it, so the default (list rows, beside 12.5pt captions) is smaller: the box
 * stays within the caption's line and its letters near the caption's capitals.
 */
export function CertificationBadge({ rating, size = 10 }: { rating: string; size?: number }) {
  const { colors } = useTheme();
  const color = colors.textMuted;
  const em = size;
  return (
    <View
      accessible
      accessibilityLabel={`Rated ${rating}`}
      style={[
        styles.box,
        {
          borderColor: color,
          borderWidth: em * STROKE,
          borderRadius: em * CORNER,
          // tides draws the border inside its padding; React Native adds it
          // outside, so the clear space is the padding less the stroke.
          paddingHorizontal: em * (PADDING_X - STROKE),
          paddingVertical: em * (PADDING_Y - STROKE),
        },
      ]}
    >
      <Text style={{ color, fontFamily: fonts.sansSemi, fontSize: em, lineHeight: em * LINE }}>
        {certificationRuns(rating).map((r, i) =>
          r.cert ? (
            // A plain run, so it inherits the badge's size instead of a Text variant's.
            <RNText key={i} style={{ fontFamily: fonts.certification }}>
              {r.text}
            </RNText>
          ) : (
            r.text
          ),
        )}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  box: { alignSelf: 'flex-start', justifyContent: 'center' },
});
