import { certificationRuns } from '@/components/CertificationBadge';

test('certificationRuns sets ratings in the certification font', () => {
  expect(certificationRuns('PG-13')).toEqual([{ text: 'PG-13', cert: true }]);
  // '/' and '+' are drawn into the font (tools/cert-font-glyphs.py).
  expect(certificationRuns('U/A 16+')).toEqual([{ text: 'U/A 16+', cert: true }]);
});

test('certificationRuns leaves characters the font lacks in the surrounding font', () => {
  expect(certificationRuns('Ö12')).toEqual([
    { text: 'Ö', cert: false },
    { text: '12', cert: true },
  ]);
  expect(certificationRuns('15歳')).toEqual([
    { text: '15', cert: true },
    { text: '歳', cert: false },
  ]);
});
