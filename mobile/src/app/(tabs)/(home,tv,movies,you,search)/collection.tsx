import { LibraryView } from '@/screens/LibraryView';

/** The library on its own (a link to /collection); in the app it is You's Library page. */
export default function Collection() {
  return <LibraryView standalone />;
}
