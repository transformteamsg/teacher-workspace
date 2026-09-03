import { lazy, Suspense } from 'react';
import { BrowserRouter, Outlet, Route, Routes } from 'react-router';

import { AppSidebar } from '~/components/Sidebar';
import { SidebarInset, SidebarProvider, SidebarTrigger } from '~/components/ui/sidebar';
import { TooltipProvider } from '~/components/ui/tooltip';
import { LoginView } from '~/containers/LoginView';
import { NotFoundView } from '~/containers/NotFoundView';

const HomeView = lazy(() => import('~/containers/HomeView'));
const StudentsView = lazy(() => import('~/containers/StudentsView'));
const PostsView = lazy(() => import('~/containers/PostsView'));
const GroupsView = lazy(() => import('~/containers/GroupsView'));

function WorkspaceLayout() {
  return (
    <SidebarProvider>
      <AppSidebar />
      <SidebarInset>
        <header className="tw:flex tw:h-14 tw:items-center tw:px-4 tw:md:hidden">
          <SidebarTrigger />
        </header>
        <Suspense fallback={null}>
          <Outlet />
        </Suspense>
      </SidebarInset>
    </SidebarProvider>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <TooltipProvider>
        <Routes>
          <Route path="/login" element={<LoginView />} />
          <Route element={<WorkspaceLayout />}>
            <Route path="/" element={<HomeView />} />
            <Route path="/students/*" element={<StudentsView />} />
            <Route path="/posts/*" element={<PostsView />} />
            <Route path="/groups/*" element={<GroupsView />} />
            <Route path="*" element={<NotFoundView />} />
          </Route>
        </Routes>
      </TooltipProvider>
    </BrowserRouter>
  );
}
