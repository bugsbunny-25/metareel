import { type Fold, getFold, isBookSplit, isTabletop } from 'fold-info';

const fold = (f: Partial<Fold>): Fold => ({
  left: 420, top: 0, width: 0, height: 900, orientation: 'vertical', state: 'flat', separating: false, occludes: false, ...f,
});

test('no native module (tests, iOS): no fold', () => {
  expect(getFold()).toBeNull();
});

test('postures', () => {
  expect(isBookSplit(fold({ separating: true }))).toBe(true); // half-open book, or a gap hinge
  expect(isBookSplit(fold({}))).toBe(false); // flat, seamless: one big screen
  expect(isBookSplit(null)).toBe(false);
  expect(isTabletop(fold({ orientation: 'horizontal', state: 'halfOpened', top: 400 }))).toBe(true);
  expect(isTabletop(fold({ orientation: 'horizontal', state: 'flat' }))).toBe(false);
  expect(isTabletop(fold({ state: 'halfOpened' }))).toBe(false); // vertical: book, not tabletop
});
