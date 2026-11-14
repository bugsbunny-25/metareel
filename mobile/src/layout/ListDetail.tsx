import { type ComponentRef, type ReactNode, useRef, useState } from 'react';
import { StyleSheet, useWindowDimensions, View } from 'react-native';

import { isBookSplit, useFold } from 'fold-info';

import { useTheme } from '@/theme';

import { useSideInsets } from './root-header';
import { useWindowClass } from './window-class';

/**
 * A list and the selected item side by side on medium and expanded windows
 * (an unfolded foldable, a tablet, a wide split-screen); just the list on
 * compact ones, where picking an item pushes its screen instead. Only the
 * layout changes when the window does, so the list keeps its scroll
 * position and the selection survives folding and unfolding.
 */
export function ListDetail({ list, detail, placeholder }: { list: ReactNode; detail: ReactNode | null; placeholder: ReactNode }) {
  const wc = useWindowClass();
  const { width } = useWindowDimensions();
  const { colors } = useTheme();
  const fold = useFold();
  // iOS: clear of iPhone Duo's control strip (and a landscape cutout) on
  // both panes, not only the list.
  const side = useSideInsets();
  // Where this layout starts in the window (after the navigation rail): the
  // fold's position is in window coordinates.
  const ref = useRef<ComponentRef<typeof View>>(null);
  const [originX, setOriginX] = useState(0);
  // On a book-style foldable the split sits on the hinge: the list on one
  // panel, the detail on the other, nothing under the fold.
  const onHinge = wc !== 'compact' && isBookSplit(fold) && fold.left - originX - (side.paddingLeft ?? 0) > 240;
  const listWidth = onHinge
    ? fold.left - originX - (side.paddingLeft ?? 0)
    : wc === 'expanded'
      ? Math.min(420, Math.round(width * 0.4))
      : Math.max(300, Math.round(width * 0.45));
  return (
    <View ref={ref} style={[styles.row, side]} onLayout={() => ref.current?.measureInWindow((x: number) => setOriginX(Math.round(x)))}>
      <View style={wc === 'compact' ? styles.full : { width: listWidth }}>{list}</View>
      {wc === 'compact' ? null : (
        <View style={[styles.detail, onHinge ? { marginLeft: fold.width, borderLeftWidth: 0 } : { borderLeftColor: colors.borderSubtle }]}>
          {detail ?? placeholder}
        </View>
      )}
    </View>
  );
}

/** True when picking an item should show it beside the list rather than push it. */
export function useTwoPane(): boolean {
  return useWindowClass() !== 'compact';
}

const styles = StyleSheet.create({
  row: { flex: 1, flexDirection: 'row' },
  full: { flex: 1 },
  detail: { flex: 1, borderLeftWidth: StyleSheet.hairlineWidth },
});
