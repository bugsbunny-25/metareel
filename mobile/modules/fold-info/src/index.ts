import { requireOptionalNativeModule } from 'expo';
import { useSyncExternalStore } from 'react';

/**
 * A fold the app's window spans, in dp relative to the window. Android only
 * (Jetpack WindowManager); null elsewhere and while no fold is spanned.
 */
export type Fold = {
  left: number;
  top: number;
  width: number;
  height: number;
  /** vertical: a book-style fold (left | right); horizontal: a clamshell or tabletop (top / bottom). */
  orientation: 'vertical' | 'horizontal';
  state: 'flat' | 'halfOpened';
  /** The fold splits the window into two areas content should not straddle. */
  separating: boolean;
  /** The hinge hides content (a physical gap); nothing should be drawn under it. */
  occludes: boolean;
};

type FoldInfoModule = {
  getFold(): Fold | null;
  addListener(event: 'foldChange', listener: (e: { fold: Fold | null }) => void): { remove(): void };
};

const native = requireOptionalNativeModule<FoldInfoModule>('FoldInfo');

let current: Fold | null = native?.getFold() ?? null;
const listeners = new Set<() => void>();
native?.addListener('foldChange', (e) => {
  current = e.fold;
  listeners.forEach((l) => l());
});

export function getFold(): Fold | null {
  return current;
}

/** The current fold; re-renders when the device folds, unfolds or changes posture. */
export function useFold(): Fold | null {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    () => current,
  );
}

/** Tabletop: a horizontal fold, half open (a laptop-like L shape). */
export function isTabletop(f: Fold | null): f is Fold {
  return !!f && f.orientation === 'horizontal' && f.state === 'halfOpened';
}

/** A book-style fold that splits the window into left and right areas. */
export function isBookSplit(f: Fold | null): f is Fold {
  return !!f && f.orientation === 'vertical' && f.separating;
}
