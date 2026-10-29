import React, { useRef } from 'react';
import { Outlet } from 'react-router';

import { ProfileMenu } from '~/components/ProfileMenu';
import { AppSidebar } from '~/components/Sidebar';
import { SidebarInset, SidebarProvider, SidebarTrigger } from '~/components/ui/sidebar';
import { WelcomeModal } from '~/components/WelcomeModal';
import { cn } from '~/helpers/cn';
import { useIsWithinViewport } from '~/hooks/use-is-within-viewport';

export default function RootLayout() {
  const topbarRef = useRef<HTMLDivElement>(null);
  const isWithinViewport = useIsWithinViewport(topbarRef);

  return (
    <SidebarProvider>
      <AppSidebar />
      <SidebarInset>
        <header
          className={cn(
            'tw:sticky tw:top-0 tw:z-30 tw:flex tw:h-14 tw:items-center tw:justify-between tw:bg-background tw:px-2 tw:transition-shadow tw:duration-200',
            isWithinViewport
              ? 'tw:shadow-[inset_0_-1px_0_0_transparent]'
              : 'tw:shadow-[inset_0_-1px_0_0_var(--border)]',
          )}
        >
          <div className="tw:flex tw:items-center tw:gap-2">
            <SidebarTrigger />
          </div>
          <div className="tw:flex tw:items-center tw:gap-2">
            <ProfileMenu />
          </div>
        </header>

        <div ref={topbarRef} className="tw:absolute tw:inset-x-0 tw:top-0 tw:h-px" />

        {/* A remote may fill its pane with `absolute inset-0`. Positioning the pane here keeps
            that below the header: against the inset, the header would cover the remote's top. */}
        <div className="tw:relative tw:flex tw:flex-1 tw:flex-col">
          <React.Suspense fallback={null}>
            <Outlet />
          </React.Suspense>
        </div>
      </SidebarInset>
      <WelcomeModal />
    </SidebarProvider>
  );
}
