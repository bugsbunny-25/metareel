import { router, useLocalSearchParams } from 'expo-router';

import { searchAction } from '@/layout/root-header';
import { TabbedRoot } from '@/layout/TabbedRoot';
import { usePersisted } from '@/lib/persisted';
import { DiaryView } from '@/screens/DiaryView';
import { LibraryView } from '@/screens/LibraryView';
import { YouView } from '@/screens/YouView';

type Page = 'you' | 'diary' | 'library';
const PAGES: Page[] = ['you', 'diary', 'library'];

/**
 * The You tab: You (profile and statistics), Diary (every watch) and Library
 * (your discs on their shelves), switched by tabs under the title. The page
 * last open comes back; /you?page=diary opens one directly.
 */
export default function YouTab() {
  const params = useLocalSearchParams<{ page?: string }>();
  const [saved, setSaved] = usePersisted<Page>('tabs.you.page', 'you', PAGES);
  const page = PAGES.includes(params.page as Page) ? (params.page as Page) : saved;
  const open = (p: Page) => {
    setSaved(p);
    if (params.page) router.setParams({ page: undefined });
  };
  return (
    <TabbedRoot
      title="You"
      actions={[{ label: 'Settings', sf: 'gearshape', icon: 'settings', onPress: () => router.push('/settings') }, searchAction]}
      segments={[
        { key: 'you', label: 'You' },
        { key: 'diary', label: 'Diary' },
        { key: 'library', label: 'Library' },
      ]}
      segment={page}
      onSegment={open}
    >
      {page === 'you' ? <YouView onOpen={open} /> : page === 'diary' ? <DiaryView /> : <LibraryView />}
    </TabbedRoot>
  );
}
