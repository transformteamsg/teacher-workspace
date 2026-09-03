import { Suspense } from 'react';
import { Outlet } from 'react-router';

import { AppSidebar } from '~/components/Sidebar';
import { SidebarInset, SidebarProvider, SidebarTrigger } from '~/components/ui/sidebar';

export default function RootLayout() {
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
