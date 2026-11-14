import { useWindowDimensions } from 'react-native';

/**
 * Material window size classes by the current window width in dp. Layouts
 * key off these, never off the device model, so the same code handles a
 * folded or unfolded foldable, split-screen, freeform windows and tablets.
 */
export type WindowClass = 'compact' | 'medium' | 'expanded';

export function windowClass(width: number): WindowClass {
  if (width < 600) return 'compact';
  if (width < 840) return 'medium';
  return 'expanded';
}

/** The current window class; re-renders when the window is resized or folded. */
export function useWindowClass(): WindowClass {
  const { width } = useWindowDimensions();
  return windowClass(width);
}
