import { useSyncExternalStore } from 'react';

const TABLET_QUERY = '(768px <= width < 1024px)';

function subscribe(onChange: () => void) {
  const mql = window.matchMedia(TABLET_QUERY);
  mql.addEventListener('change', onChange);
  return () => mql.removeEventListener('change', onChange);
}

function getSnapshot() {
  return window.matchMedia(TABLET_QUERY).matches;
}

export function useIsTablet() {
  return useSyncExternalStore(subscribe, getSnapshot);
}
