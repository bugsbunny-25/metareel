import { breaksAWord } from '@/lib/line-breaks';

test('breaksAWord spots a word split across lines', () => {
  expect(breaksAWord(['Dexter: ', 'Resurrectio', 'n'])).toBe(true);
  expect(breaksAWord(['Dexter: ', 'Resurrection'])).toBe(false);
  expect(breaksAWord(['Inception'])).toBe(false);
  expect(breaksAWord(['Spider-', 'Man'])).toBe(false);
  expect(breaksAWord(['Amélie and ', 'Bérénice'])).toBe(false);
  expect(breaksAWord(['Améli', 'e'])).toBe(true);
});

test('breaksAWord leaves scripts without spaces alone', () => {
  expect(breaksAWord(['千と千尋の', '神隠し'])).toBe(false);
});
