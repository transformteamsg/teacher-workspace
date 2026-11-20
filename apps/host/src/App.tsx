import React from 'react';
import { BrowserRouter, Route, Routes } from 'react-router';

import { ErrorBoundary } from '~/components/ErrorBoundary';
import { Toaster } from '~/components/ui/toast';
import { TooltipProvider } from '~/components/ui/tooltip';
import { NotFoundView } from '~/containers/NotFoundView';
import { type RemoteApp, remoteApps } from '~/remote-apps';
import { isAppAllowed } from '~/stores/preloaded-state';

const LoginView = React.lazy(() => import('~/containers/LoginView'));
const RootLayout = React.lazy(() => import('~/containers/RootLayout'));
const HomeView = React.lazy(() => import('~/containers/HomeView'));
const RemoteLoadFallbackView = React.lazy(() => import('~/containers/RemoteLoadFallbackView'));

// A refused app's route falls through to the 404 page, so its remote never loads.
function remoteRoute({ key, path, view: View }: RemoteApp) {
  if (View === undefined || !isAppAllowed(key)) {
    return null;
  }

  return (
    <Route
      key={key}
      path={`${path}/*`}
      element={
        <ErrorBoundary
          key={key}
          fallback={<RemoteLoadFallbackView onRetry={() => window.location.reload()} />}
        >
          <React.Suspense fallback={null}>
            <View />
          </React.Suspense>
        </ErrorBoundary>
      }
    />
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <TooltipProvider>
        <Toaster />

        <Routes>
          <Route path="/login" element={<LoginView />} />
          <Route element={<RootLayout />}>
            <Route path="/" element={<HomeView />} />
            {remoteApps.map(remoteRoute)}
          </Route>
          <Route path="*" element={<NotFoundView />} />
        </Routes>
      </TooltipProvider>
    </BrowserRouter>
  );
}
