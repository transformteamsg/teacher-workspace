import { lazy, Suspense } from 'react';
import { BrowserRouter, Route, Routes } from 'react-router';

import { Toaster } from '~/components/ui/sonner';
import { TooltipProvider } from '~/components/ui/tooltip';
import { LoginView } from '~/containers/LoginView';
import { NotFoundView } from '~/containers/NotFoundView';

const RootLayout = lazy(() => import('~/containers/RootLayout'));
const HomeView = lazy(() => import('~/containers/HomeView'));
const StudentsView = lazy(() => import('~/containers/StudentsView'));
const PostsView = lazy(() => import('~/containers/PostsView'));
const GroupsView = lazy(() => import('~/containers/GroupsView'));

export default function App() {
  return (
    <BrowserRouter>
      <TooltipProvider>
        <Toaster position="bottom-right" />
        <Suspense fallback={null}>
          <Routes>
            <Route path="/login" element={<LoginView />} />
            <Route element={<RootLayout />}>
              <Route path="/" element={<HomeView />} />
              <Route path="/students/*" element={<StudentsView />} />
              <Route path="/posts/*" element={<PostsView />} />
              <Route path="/groups/*" element={<GroupsView />} />
              <Route path="*" element={<NotFoundView />} />
            </Route>
          </Routes>
        </Suspense>
      </TooltipProvider>
    </BrowserRouter>
  );
}
