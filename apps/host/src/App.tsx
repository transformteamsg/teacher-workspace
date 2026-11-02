import { loadRemote } from '@module-federation/enhanced/runtime';
import React from 'react';
import { BrowserRouter, Route, Routes } from 'react-router';

import { ErrorBoundary } from '~/components/ErrorBoundary';
import { Toaster } from '~/components/ui/toast';
import { TooltipProvider } from '~/components/ui/tooltip';
import { NotFoundView } from '~/containers/NotFoundView';

const LoginView = React.lazy(() => import('~/containers/LoginView'));
const RootLayout = React.lazy(() => import('~/containers/RootLayout'));
const HomeView = React.lazy(() => import('~/containers/HomeView'));
const RemoteLoadFallbackView = React.lazy(() => import('~/containers/RemoteLoadFallbackView'));

const RemoteStudentInsightsView = React.lazy(async () => {
  const module = await loadRemote<{ default: React.ComponentType }>('student_insights/App');
  if (!module) {
    throw new Error('Failed to load remote module');
  }

  return module;
});

const RemotePostsView = React.lazy(async () => {
  const module = await loadRemote<{ default: React.ComponentType }>('pg/Posts');
  if (!module) {
    throw new Error('Failed to load remote module');
  }

  return module;
});

const RemoteGroupsView = React.lazy(async () => {
  const module = await loadRemote<{ default: React.ComponentType }>('pg/Groups');
  if (!module) {
    throw new Error('Failed to load remote module');
  }

  return module;
});

export default function App() {
  return (
    <BrowserRouter>
      <TooltipProvider>
        <Toaster />

        <Routes>
          <Route path="/login" element={<LoginView />} />
          <Route element={<RootLayout />}>
            <Route path="/" element={<HomeView />} />
            <Route
              path="/students/*"
              element={
                <ErrorBoundary
                  key="students"
                  fallback={<RemoteLoadFallbackView onRetry={() => window.location.reload()} />}
                >
                  <React.Suspense fallback={null}>
                    <RemoteStudentInsightsView />
                  </React.Suspense>
                </ErrorBoundary>
              }
            />
            <Route
              path="/posts/*"
              element={
                <ErrorBoundary
                  key="posts"
                  fallback={<RemoteLoadFallbackView onRetry={() => window.location.reload()} />}
                >
                  <React.Suspense fallback={null}>
                    <RemotePostsView />
                  </React.Suspense>
                </ErrorBoundary>
              }
            />
            <Route
              path="/groups/*"
              element={
                <ErrorBoundary
                  key="groups"
                  fallback={<RemoteLoadFallbackView onRetry={() => window.location.reload()} />}
                >
                  <React.Suspense fallback={null}>
                    <RemoteGroupsView />
                  </React.Suspense>
                </ErrorBoundary>
              }
            />
          </Route>
          <Route path="*" element={<NotFoundView />} />
        </Routes>
      </TooltipProvider>
    </BrowserRouter>
  );
}
