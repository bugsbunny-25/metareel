import { windowClass } from '@/layout/window-class';
import { resolveScheme } from '@/theme';
import { tokens } from '@/theme/tokens';

test('window classes follow the Material breakpoints', () => {
  expect(windowClass(360)).toBe('compact'); // phone, Fold cover screen
  expect(windowClass(599)).toBe('compact');
  expect(windowClass(600)).toBe('medium'); // unfolded book foldable, small tablet
  expect(windowClass(839)).toBe('medium');
  expect(windowClass(840)).toBe('expanded');
});

test('the theme is dark first', () => {
  expect(resolveScheme('system', null)).toBe('dark');
  expect(resolveScheme('system', 'dark')).toBe('dark');
  expect(resolveScheme('system', 'light')).toBe('light');
  expect(resolveScheme('light', 'dark')).toBe('light');
  expect(Object.keys(tokens.dark)).toEqual(Object.keys(tokens.light));
  expect(tokens.dark.accentYours).toBe('#ff8a4c');
});
