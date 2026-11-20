import { loadRemote } from '@module-federation/enhanced/runtime';
import React from 'react';
import { BrowserRouter, Route, Routes } from 'react-router';

import { ErrorBoundary } from '~/components/ErrorBoundary';
import { Toaster } from '~/components/ui/toast';
import { TooltipProvider } from '~/components/ui/tooltip';
import { NotFoundView } from '~/containers/NotFoundView';
import { type AppKey, isAppAllowed } from '~/stores/preloaded-state';

const LoginView = React.lazy(() => import('~/containers/LoginView'));
const RootLayout = React.lazy(() => import('~/containers/RootLayout'));
const HomeView = React.lazy(() => import('~/containers/HomeView'));
const RemoteLoadFallbackView = React.lazy(() => import('~/containers/RemoteLoadFallbackView'));

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

const RemoteStudentInsightsView = remoteView('student_insights/App');
const RemotePostsView = remoteView('pg/Posts');
const RemoteGroupsView = remoteView('pg/Groups');

// A refused app's route falls through to the 404 page, so its remote never loads.
function remoteRoute(app: AppKey, path: string, View: React.ComponentType) {
  if (!isAppAllowed(app)) {
    return null;
  }

  return (
    <Route
      path={path}
      element={
        <ErrorBoundary
          key={app}
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
            {remoteRoute('students', '/students/*', RemoteStudentInsightsView)}
            {remoteRoute('posts', '/posts/*', RemotePostsView)}
            {remoteRoute('groups', '/groups/*', RemoteGroupsView)}
          </Route>
          <Route path="*" element={<NotFoundView />} />
        </Routes>
      </TooltipProvider>
    </BrowserRouter>
  );
}
