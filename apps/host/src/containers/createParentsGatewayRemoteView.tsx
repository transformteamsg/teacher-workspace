import { loadRemote, registerRemotes } from '@module-federation/enhanced/runtime';
import React, { Suspense } from 'react';

import { ErrorBoundary } from '~/components/ErrorBoundary';

registerRemotes([
  {
    name: 'pg',
    entry: 'https://d390008ekba73v.cloudfront.net/mf-manifest.json',
  },
]);

export function createParentsGatewayRemoteView(moduleId: string, unavailableLabel: string) {
  const RemoteApp = React.lazy(async () => {
    const module = await loadRemote<{ default: React.ComponentType }>(moduleId);

    if (!module) {
      throw new Error(`Failed to load ${moduleId} remote module`);
    }

    return module;
  });

  function Fallback() {
    return (
      <div className="tw:flex tw:flex-1 tw:items-center tw:justify-center tw:p-8">
        <h1 className="tw:text-2xl tw:font-semibold tw:text-muted-foreground">
          {unavailableLabel} is unavailable right now
        </h1>
      </div>
    );
  }

  return function RemoteView() {
    return (
      <ErrorBoundary fallback={<Fallback />}>
        <Suspense fallback={null}>
          <RemoteApp />
        </Suspense>
      </ErrorBoundary>
    );
  };
}
