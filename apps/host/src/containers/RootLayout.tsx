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
        <header className="tw:sticky tw:top-0 tw:z-30 tw:flex tw:h-14 tw:items-center tw:justify-between tw:bg-background tw:px-2">
          <div className="tw:flex tw:items-center tw:gap-2">
            <SidebarTrigger />
          </div>
          <div className="tw:flex tw:items-center tw:gap-2">
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
