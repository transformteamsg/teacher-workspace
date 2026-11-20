import { loadRemote } from '@module-federation/enhanced/runtime';
import React from 'react';

import type { AppKey } from '~/stores/preloaded-state';

/**
 * An app the host offers when the server allows it.
 */
export interface RemoteApp {
  key: AppKey;
  /** Where the app lives in the host, such as "/posts". */
  path: string;
  /** The remote module the app's route renders, or none while the app has no route. */
  view?: React.ComponentType;
}

// Loads a remote's exposed module, such as 'pg/Posts', as a lazy view.
// Call it at module level: a lazy component created during render reloads on every render.
function remoteView(id: string) {
  return React.lazy(async () => {
    const module = await loadRemote<{ default: React.ComponentType }>(id);
    if (!module) {
      throw new Error('Failed to load remote module');
    }

    return module;
  });
}

/** Every app the host can offer. */
export const remoteApps: readonly RemoteApp[] = [
  { key: 'students', path: '/students', view: remoteView('student_insights/App') },
  { key: 'posts', path: '/posts', view: remoteView('pg/Posts') },
  { key: 'meetings', path: '/meetings' },
  { key: 'groups', path: '/groups', view: remoteView('pg/Groups') },
];
