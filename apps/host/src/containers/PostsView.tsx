import { loadRemote, registerRemotes } from '@module-federation/enhanced/runtime';
import React from 'react';

import { ErrorBoundary } from '~/components/ErrorBoundary';

registerRemotes([
  {
    name: 'pg',
    entry: 'https://d390008ekba73v.cloudfront.net/mf-manifest.json',
  },
]);

const RemoteApp = React.lazy(async () => {
  const module = await loadRemote<{ default: React.ComponentType }>('pg/Posts');

  if (!module) {
    throw new Error('Failed to load pg/Posts remote module');
  }

  return module;
});

function Fallback() {
  return (
    <div className="tw:flex tw:flex-1 tw:items-center tw:justify-center tw:p-8">
      <h1 className="tw:text-2xl tw:font-semibold tw:text-muted-foreground">
        Parents Gateway is unavailable right now
      </h1>
    </div>
  );
}

export default function PostsView() {
  return (
    <ErrorBoundary fallback={<Fallback />}>
      <React.Suspense fallback={null}>
        <RemoteApp />
      </React.Suspense>
    </ErrorBoundary>
  );
}
