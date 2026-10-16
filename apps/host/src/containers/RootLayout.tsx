import React from 'react';
import { Outlet } from 'react-router';

import { ProfileMenu } from '~/components/ProfileMenu';
import { AppSidebar } from '~/components/Sidebar';
import { SidebarInset, SidebarProvider, SidebarTrigger } from '~/components/ui/sidebar';
import { WelcomeModal } from '~/components/WelcomeModal';

export default function RootLayout() {
  return (
    <SidebarProvider>
      <AppSidebar />
      <SidebarInset>
        <header className="tw:flex tw:h-14 tw:items-center tw:px-4">
          <SidebarTrigger className="tw:md:hidden" />
          <div className="tw:ml-auto">
            <ProfileMenu />
          </div>
        </header>
        <React.Suspense fallback={null}>
          <Outlet />
        </React.Suspense>
      </SidebarInset>
      <WelcomeModal />
    </SidebarProvider>
  );
}
