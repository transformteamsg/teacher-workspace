import { CalendarClock, CircleHelp, Home, Layers, Mail, Users } from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import { NavLink, useMatch } from 'react-router';

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
        <span>{item.title}</span>
      </SidebarMenuButton>
    </SidebarMenuItem>
  );
}

export function AppSidebar() {
  return (
    <SidebarRoot collapsible="icon">
      <SidebarHeader className="tw:p-0">
        <div className="tw:flex tw:h-14 tw:items-center tw:justify-center tw:gap-2 tw:px-4 tw:group-data-[collapsible=icon]:gap-0 tw:group-data-[collapsible=icon]:px-0">
          <span className="tw:min-w-0 tw:flex-1 tw:cursor-default tw:truncate tw:text-sm tw:font-semibold tw:transition-[opacity,flex] tw:duration-150 tw:select-none tw:group-data-[collapsible=icon]:flex-[0] tw:group-data-[collapsible=icon]:opacity-0">
            Teacher Workspace
            <span className="tw:ml-1.5 tw:rounded-full tw:bg-[#eaf3ff] tw:px-1.5 tw:py-0.5 tw:text-xs tw:font-medium tw:text-primary">
              Beta
            </span>
          </span>
          <SidebarTrigger />
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
              <span>Help</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
    </SidebarRoot>
  );
}
