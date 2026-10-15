import { CalendarClock, CircleHelp, Home, Layers, Mail, Users } from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import { useState } from 'react';
import type { CSSProperties, ReactNode } from 'react';
import { NavLink, useMatch } from 'react-router';

import teacherWorkspaceMark from '~/assets/logos/teacher-workspace.svg';
import {
  Sidebar as SidebarRoot,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  useSidebar,
} from '~/components/ui/sidebar';
import { cn } from '~/helpers/cn';
import { useIsTablet } from '~/hooks/use-tablet';

interface NavItem {
  title: string;
  icon: LucideIcon;
  to: string;
}

interface NavGroup {
  label?: string;
  items: NavItem[];
}

const navGroups: NavGroup[] = [
  {
    items: [{ title: 'Home', icon: Home, to: '/' }],
  },
  {
    label: 'Student Insights',
    items: [{ title: 'Profiles', icon: Users, to: '/students' }],
  },
  {
    label: 'Communications',
    items: [
      { title: 'Posts', icon: Mail, to: '/posts' },
      { title: 'Meetings', icon: CalendarClock, to: '/meetings' },
    ],
  },
  {
    label: 'Manage',
    items: [{ title: 'Student groups', icon: Layers, to: '/groups' }],
  },
];

const DESKTOP_OPEN_KEY = 'tw_sidebar_desktop_open';
const TABLET_OPEN_KEY = 'tw_sidebar_tablet_open';

// shadcn centres a collapsed item with flex, and justify-content cannot
// transition, so the icon would drift while the rail narrows.
const MENU_ITEM_CLASS = 'tw:group-data-[collapsible=icon]:block';
const MENU_BUTTON_CLASS =
  'tw:h-9 tw:gap-3 tw:rounded-xl tw:transition-[width,height,padding,margin] tw:duration-(--duration-sidebar) tw:ease-(--ease-sidebar) tw:group-data-[collapsible=icon]:my-0.5';
const EXPANDED_ONLY_TEXT_CLASS =
  'tw:transition-[opacity,filter] tw:delay-75 tw:duration-150 tw:ease-out tw:group-data-[collapsible=icon]:opacity-0 tw:group-data-[collapsible=icon]:blur-sm tw:group-data-[collapsible=icon]:delay-0 tw:group-data-[collapsible=icon]:duration-100 tw:motion-reduce:transition-opacity tw:motion-reduce:delay-0 tw:motion-reduce:duration-100 tw:motion-reduce:group-data-[collapsible=icon]:blur-none';

function readStoredOpen(key: string, fallback: boolean): boolean {
  try {
    const value = window.localStorage.getItem(key);
    if (value === 'true') return true;
    if (value === 'false') return false;
    return fallback;
  } catch {
    return fallback;
  }
}

function storeOpen(key: string, open: boolean) {
  try {
    window.localStorage.setItem(key, String(open));
  } catch {
    return;
  }
}

export function AppSidebarProvider({ children }: { children: ReactNode }) {
  const isTablet = useIsTablet();
  const [desktopOpen, setDesktopOpen] = useState(() => readStoredOpen(DESKTOP_OPEN_KEY, true));
  const [tabletOpen, setTabletOpen] = useState(() => readStoredOpen(TABLET_OPEN_KEY, false));

  const handleOpenChange = (open: boolean) => {
    if (isTablet) {
      setTabletOpen(open);
      storeOpen(TABLET_OPEN_KEY, open);
    } else {
      setDesktopOpen(open);
      storeOpen(DESKTOP_OPEN_KEY, open);
    }
  };

  return (
    <SidebarProvider
      open={isTablet ? tabletOpen : desktopOpen}
      onOpenChange={handleOpenChange}
      style={{ '--sidebar-width-icon': '3rem' } as CSSProperties}
    >
      {children}
    </SidebarProvider>
  );
}

function NavMenuItem({ item }: { item: NavItem }) {
  const { setOpenMobile } = useSidebar();
  const end = item.to === '/';
  const isActive = useMatch({ path: item.to, end }) !== null;

  return (
    <SidebarMenuItem className={MENU_ITEM_CLASS}>
      <SidebarMenuButton
        render={<NavLink to={item.to} end={end} />}
        isActive={isActive}
        tooltip={item.title}
        className={MENU_BUTTON_CLASS}
        onClick={() => setOpenMobile(false)}
      >
        <item.icon />
        <span className={EXPANDED_ONLY_TEXT_CLASS}>{item.title}</span>
      </SidebarMenuButton>
    </SidebarMenuItem>
  );
}

export function AppSidebar() {
  return (
    <SidebarRoot
      collapsible="icon"
      className="tw:border-sidebar-border tw:duration-(--duration-sidebar) tw:ease-(--ease-sidebar)"
    >
      <SidebarHeader className="tw:in-data-[slot=sidebar-inner]:mb-2">
        <div className="tw:flex tw:h-10 tw:items-center tw:gap-2 tw:pl-1 tw:transition-[gap] tw:duration-(--duration-sidebar) tw:ease-(--ease-sidebar) tw:select-none tw:group-data-[collapsible=icon]:gap-0">
          <img
            src={teacherWorkspaceMark}
            alt=""
            draggable={false}
            className="tw:size-6 tw:shrink-0"
          />
          <span
            className={cn(
              'tw:min-w-0 tw:flex-1 tw:truncate tw:text-[13px] tw:font-semibold tw:group-data-[collapsible=icon]:flex-[0]',
              EXPANDED_ONLY_TEXT_CLASS,
            )}
          >
            Teacher Workspace
          </span>
          <span
            className={cn(
              'tw:inline-flex tw:h-5 tw:shrink-0 tw:items-center tw:overflow-hidden tw:rounded-4xl tw:border tw:border-transparent tw:bg-info tw:px-2 tw:py-0.5 tw:text-xs tw:font-medium tw:text-info-foreground tw:group-data-[collapsible=icon]:w-0 tw:group-data-[collapsible=icon]:border-0 tw:group-data-[collapsible=icon]:px-0',
              EXPANDED_ONLY_TEXT_CLASS,
            )}
          >
            Beta
          </span>
        </div>
      </SidebarHeader>

      <SidebarContent className="tw:transition-[gap] tw:duration-(--duration-sidebar) tw:ease-(--ease-sidebar) tw:group-data-[collapsible=icon]:gap-1">
        {navGroups.map((group, index) => (
          <SidebarGroup
            key={group.label ?? index}
            className={cn(
              'tw:transition-[padding,margin] tw:duration-(--duration-sidebar) tw:ease-(--ease-sidebar) tw:group-data-[collapsible=icon]:pb-0',
              group.label &&
                'tw:group-data-[collapsible=icon]:mt-2 tw:group-data-[collapsible=icon]:pt-0',
            )}
          >
            {group.label && (
              // A label slides over the row above it while the sidebar expands, so
              // it takes no pointer events and a click there reaches that row.
              <SidebarGroupLabel className="tw:pointer-events-none tw:transition-[margin,opacity,filter] tw:delay-[0s,75ms,75ms] tw:duration-[var(--duration-sidebar),150ms,150ms] tw:ease-[var(--ease-sidebar),ease-out,ease-out] tw:group-data-[collapsible=icon]:blur-sm tw:group-data-[collapsible=icon]:delay-0 tw:group-data-[collapsible=icon]:duration-[var(--duration-sidebar),100ms,100ms] tw:motion-reduce:delay-0 tw:motion-reduce:duration-[var(--duration-sidebar),100ms,100ms] tw:motion-reduce:group-data-[collapsible=icon]:blur-none">
                {group.label}
              </SidebarGroupLabel>
            )}
            <SidebarGroupContent>
              <SidebarMenu>
                {group.items.map((item) => (
                  <NavMenuItem key={item.to} item={item} />
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        ))}
      </SidebarContent>

      <SidebarFooter className="tw:in-data-[slot=sidebar-inner]:mt-2">
        <SidebarMenu>
          <SidebarMenuItem className={MENU_ITEM_CLASS}>
            <SidebarMenuButton
              render={
                <a
                  href="https://go.gov.sg/teacherworkspace-feedback"
                  target="_blank"
                  rel="noopener noreferrer"
                />
              }
              tooltip="Help"
              className={MENU_BUTTON_CLASS}
            >
              <CircleHelp />
              <span className={EXPANDED_ONLY_TEXT_CLASS}>Help</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
    </SidebarRoot>
  );
}
