import { CalendarClock, CircleHelp, Home, Layers, Mail, Users } from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
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
  SidebarTrigger,
  useSidebar,
} from '~/components/ui/sidebar';
import { cn } from '~/helpers/cn';

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

// Text that exists only in the expanded sidebar. Collapsing, it fades and blurs
// out in 100ms, before the width has moved far enough to truncate it. Expanding,
// it resolves back in once the width has started to settle. With reduced motion
// it only fades.
const EXPANDED_ONLY_TEXT_CLASS =
  'tw:transition-[opacity,filter] tw:delay-75 tw:duration-150 tw:ease-out tw:group-data-[collapsible=icon]:opacity-0 tw:group-data-[collapsible=icon]:blur-sm tw:group-data-[collapsible=icon]:delay-0 tw:group-data-[collapsible=icon]:duration-100 tw:motion-reduce:transition-opacity tw:motion-reduce:delay-0 tw:motion-reduce:duration-100 tw:motion-reduce:group-data-[collapsible=icon]:blur-none';

function NavMenuItem({ item }: { item: NavItem }) {
  const { setOpenMobile } = useSidebar();
  // Home matches only itself; every other destination stays active on the
  // pages nested under it, such as one student's profile.
  const end = item.to === '/';
  const isActive = useMatch({ path: item.to, end }) !== null;

  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        render={<NavLink to={item.to} end={end} />}
        isActive={isActive}
        tooltip={item.title}
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
    <SidebarRoot collapsible="icon">
      <SidebarHeader>
        {/* The collapse control sits over the product mark and shows in its place
            on hover or keyboard focus, so the row reads as the mark, the name and
            the Beta pill, and collapses to the mark alone. On touch screens the
            mark stays visible and tapping it toggles the sidebar. */}
        <div className="tw:group/identity tw:relative tw:flex tw:h-10 tw:items-center tw:gap-2 tw:pl-1 tw:transition-[gap] tw:duration-(--duration-sidebar) tw:ease-(--ease-sidebar) tw:select-none tw:group-data-[collapsible=icon]:gap-0">
          <img
            src={teacherWorkspaceMark}
            alt=""
            draggable={false}
            className="tw:size-6 tw:shrink-0 tw:transition-opacity tw:duration-150 tw:group-hover/identity:opacity-0 tw:group-has-[:focus-visible]/identity:opacity-0"
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
              'tw:inline-flex tw:h-5 tw:shrink-0 tw:items-center tw:overflow-hidden tw:rounded-full tw:bg-info tw:px-2 tw:text-xs tw:font-medium tw:text-info-foreground tw:group-data-[collapsible=icon]:w-0 tw:group-data-[collapsible=icon]:px-0',
              EXPANDED_ONLY_TEXT_CLASS,
            )}
          >
            Beta
          </span>
          <SidebarTrigger className="tw:absolute tw:top-1 tw:left-0 tw:opacity-0 tw:transition-opacity tw:duration-150 tw:group-hover/identity:opacity-100 tw:focus-visible:opacity-100" />
        </div>
      </SidebarHeader>

      <SidebarContent>
        {navGroups.map((group, index) => (
          <SidebarGroup key={group.label ?? index}>
            {group.label && <SidebarGroupLabel>{group.label}</SidebarGroupLabel>}
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

      <SidebarFooter>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              render={
                <a
                  href="https://go.gov.sg/teacherworkspace-feedback"
                  target="_blank"
                  rel="noopener noreferrer"
                />
              }
              tooltip="Help"
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
