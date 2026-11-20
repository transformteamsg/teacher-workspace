import { loadRemote } from '@module-federation/enhanced/runtime';
import { CalendarClock, Layers, Mail, Users } from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import React from 'react';

import type { AppKey } from '~/stores/preloaded-state';

/** The sidebar's labelled groups, in the order they are shown. */
export const navGroups = ['Student Insights', 'Communications', 'Manage'] as const;

/**
 * An app the host offers when the server allows it.
 */
export interface RemoteApp {
  key: AppKey;
  /** Where the app lives in the host, such as "/posts". */
  path: string;
  /** The remote module the app's route renders, or none while the app has no route. */
  view?: React.ComponentType;
  /** The app's item in the sidebar, shown under its group in the order the apps are listed. */
  nav: { group: (typeof navGroups)[number]; title: string; icon: LucideIcon };
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
  {
    key: 'students',
    path: '/students',
    view: remoteView('student_insights/App'),
    nav: { group: 'Student Insights', title: 'Profiles', icon: Users },
  },
  {
    key: 'posts',
    path: '/posts',
    view: remoteView('pg/Posts'),
    nav: { group: 'Communications', title: 'Posts', icon: Mail },
  },
  {
    key: 'meetings',
    path: '/meetings',
    nav: { group: 'Communications', title: 'Meetings', icon: CalendarClock },
  },
  {
    key: 'groups',
    path: '/groups',
    view: remoteView('pg/Groups'),
    nav: { group: 'Manage', title: 'Student groups', icon: Layers },
  },
];
