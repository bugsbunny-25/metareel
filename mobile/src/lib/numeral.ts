// Geometry of the chart rank numerals (components/Numeral.tsx; the web's app/src/lib/numeral.js
// is the same): Barlow Condensed ExtraBold set at 100 units, tightened by TRACKING, with a slab
// foot under a lone 1 (the 1 in 10 stays plain). Measured from the font: per digit
// [advance, ink left, ink right].
const DIGITS: Record<string, [number, number, number]> = {
  0: [45.6, 2.3, 43.3],
  1: [29.4, 0.8, 26.7],
  2: [45.1, 1.9, 43.8],
  3: [44.6, 1.8, 42.2],
  4: [50.8, 1.1, 49.8],
  5: [45, 2.4, 43.1],
  6: [45.1, 2.3, 43.2],
  7: [42.2, 0.9, 41.4],
  8: [44.6, 2.3, 42.4],
  9: [44.6, 1.4, 42.3],
};
export const TRACKING = -3;
const ASCENT = 71.2; // the round digits' overshoot above the baseline
const DESCENT = 1.2;
const PAD = 3; // room for the outline
// The 1's stem runs from 10 to 26 units; the foot is centred on it.
const STEM_CENTRE = 18;
const FOOT = { half: 16, h: 12 };

type Box = { x: number; y: number; w: number; h: number };
export type NumeralLayout = { x: number; baseline: number; width: number; height: number; foot: Box | null };

export function numeralLayout(value: number): NumeralLayout {
  const s = String(value);
  let pen = 0;
  let inkL = Infinity;
  let inkR = -Infinity;
  for (const ch of s) {
    const [adv, l, r] = DIGITS[ch] ?? [45, 2, 43];
    inkL = Math.min(inkL, pen + l);
    inkR = Math.max(inkR, pen + r);
    pen += adv + TRACKING;
  }
  const r1 = (v: number) => Math.round(v * 10) / 10;
  const x = PAD - inkL;
  const baseline = PAD + ASCENT;
  let right = x + inkR;
  let foot: Box | null = null;
  if (s === '1') {
    const cx = x + STEM_CENTRE;
    foot = { x: r1(cx - FOOT.half), y: r1(baseline - FOOT.h), w: FOOT.half * 2, h: FOOT.h };
    right = Math.max(right, foot.x + foot.w);
  }
  return { x: r1(x), baseline: r1(baseline), width: r1(right + PAD), height: r1(baseline + DESCENT + PAD), foot };
}
