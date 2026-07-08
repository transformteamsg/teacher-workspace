import { loadRemote, registerRemotes } from '@module-federation/enhanced/runtime';
import React, { Suspense } from 'react';

import { ErrorBoundary } from '~/components/ErrorBoundary';

registerRemotes([
  {
    name: 'pg',
    entry: 'https://d390008ekba73v.cloudfront.net/mf-manifest.json',
  },
]);

const RemoteApp = React.lazy(async () => {
  const module = await loadRemote<{ default: React.ComponentType }>('pg/App');

  if (!module) {
    throw new Error('Failed to load Parents Gateway remote module');
  }

  return module;
});

function ParentsGatewayFallback() {
  return (
    <div className="tw:flex tw:flex-1 tw:items-center tw:justify-center tw:p-8">
      <h1 className="tw:text-2xl tw:font-semibold tw:text-muted-foreground">
        Parents Gateway is unavailable right now
      </h1>
    </div>
  );
}

export function ParentsGatewayView() {
  return (
    <ErrorBoundary fallback={<ParentsGatewayFallback />}>
      <Suspense fallback={null}>
        <RemoteApp />
      </Suspense>
    </ErrorBoundary>
  );
}
