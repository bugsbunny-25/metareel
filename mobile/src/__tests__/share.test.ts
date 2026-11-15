import { parseShared } from '@/lib/share';

test('links metareel can look up', () => {
  expect(parseShared('https://www.themoviedb.org/movie/693134-dune-part-two')).toEqual({ type: 'tmdb', kind: 'movie', id: '693134' });
  expect(parseShared('Check this out https://m.imdb.com/title/tt15239678/?ref_=ext_shr')).toEqual({ type: 'imdb', id: 'tt15239678' });
  expect(parseShared('Watch "Wednesday" on Netflix: https://www.netflix.com/title/81231974?s=a&trkid=1')).toEqual({ type: 'netflix', id: '81231974' });
  expect(parseShared('https://www.netflix.com/in-en/title/81231974')).toEqual({ type: 'netflix', id: '81231974' });
  expect(parseShared('Dune: Part Two https://boxd.it/aRvI')).toEqual({ type: 'letterboxd', url: 'https://boxd.it/aRvI' });
  expect(parseShared('https://letterboxd.com/film/dune-part-two/')).toEqual({ type: 'letterboxd', url: 'https://letterboxd.com/film/dune-part-two/' });
});

test('anything else becomes a search', () => {
  expect(parseShared('Check out Severance on Apple TV+ https://tv.apple.com/us/show/severance/umc.cmc.1srk2goyh2q2zdxcx605w8vtx')).toEqual({ type: 'search', query: 'Severance' });
  expect(parseShared('https://www.justwatch.com/us/movie/the-substance')).toEqual({ type: 'search', query: 'the substance' });
  expect(parseShared('Oppenheimer')).toEqual({ type: 'search', query: 'Oppenheimer' });
  expect(parseShared('   ')).toBeNull();
});
