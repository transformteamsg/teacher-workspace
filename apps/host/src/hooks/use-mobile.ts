import * as React from 'react';

const MOBILE_BREAKPOINT = 768;
const DESKTOP_BREAKPOINT = 1024;

// Read synchronously rather than after mount, so the first render already lays
// the sidebar out for this screen and nothing animates into place on load.
function useMediaQuery(query: string) {
  const mql = React.useMemo(() => window.matchMedia(query), [query]);
  const subscribe = React.useCallback(
    (onChange: () => void) => {
      mql.addEventListener('change', onChange);
      return () => mql.removeEventListener('change', onChange);
    },
    [mql],
  );

  return React.useSyncExternalStore(subscribe, () => mql.matches);
}

// Phone (narrower than 768px): the sidebar is an off-canvas drawer.
export function useIsMobile() {
  return useMediaQuery(`(width < ${MOBILE_BREAKPOINT}px)`);
}

// Tablet (768px to 1023px): the sidebar starts as the icon rail.
export function useIsTablet() {
  return useMediaQuery(`(${MOBILE_BREAKPOINT}px <= width < ${DESKTOP_BREAKPOINT}px)`);
}
