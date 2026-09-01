import { loadRemote, registerRemotes } from '@module-federation/enhanced/runtime';
import React from 'react';

import { ErrorBoundary } from '~/components/ErrorBoundary';
import { PG_REMOTE_ENTRY } from '~/helpers/pgRemote';

registerRemotes([
  {
    name: 'pg',
    entry: PG_REMOTE_ENTRY,
  },
]);

const RemoteApp = React.lazy(async () => {
  const module = await loadRemote<{ default: React.ComponentType }>('pg/Groups');

  if (!module) {
    throw new Error('Failed to load pg/Groups remote module');
  }

  return module;
});

function Fallback() {
  return (
    <div className="tw:flex tw:flex-1 tw:items-center tw:justify-center tw:p-8">
      <h1 className="tw:text-2xl tw:font-semibold tw:text-muted-foreground">
        Groups is unavailable right now
      </h1>
    </div>
  );
}

export default function GroupsView() {
  return (
    <ErrorBoundary fallback={<Fallback />}>
      <React.Suspense fallback={null}>
        <RemoteApp />
      </React.Suspense>
    </ErrorBoundary>
  );
}
