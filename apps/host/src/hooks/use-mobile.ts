import * as React from 'react';

const MOBILE_BREAKPOINT = 1024;
const TABLET_BREAKPOINT = 768;

// Reads the query during render, so the first render already has the right
// layout instead of flashing the desktop one.
function useMediaQuery(query: string) {
  const subscribe = React.useCallback(
    (onChange: () => void) => {
      const mql = window.matchMedia(query);
      mql.addEventListener('change', onChange);
      return () => mql.removeEventListener('change', onChange);
    },
    [query],
  );
  return React.useSyncExternalStore(subscribe, () => window.matchMedia(query).matches);
}

// The ranges use range syntax rather than `max-width: 767px` / `min-width:
// 768px`, which both miss a fractional width such as 767.5px under zoom.

// Compact (< 1024px): phones and tablets, for layouts that want a single
// breakpoint.
export function useIsMobile() {
  return useMediaQuery(`(width < ${MOBILE_BREAKPOINT}px)`);
}

// Phone (< 768px): the sidebar uses its off-canvas drawer here.
export function useIsPhone() {
  return useMediaQuery(`(width < ${TABLET_BREAKPOINT}px)`);
}

// Tablet (768px to 1023px): the sidebar shows its collapsed icon rail here.
export function useIsTablet() {
  return useMediaQuery(`(${TABLET_BREAKPOINT}px <= width < ${MOBILE_BREAKPOINT}px)`);
}
