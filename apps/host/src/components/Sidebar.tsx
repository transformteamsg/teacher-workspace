import { CalendarClock, CircleHelp, Home, Layers, Mail, Users } from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import { Link, useMatch } from 'react-router';

import TeacherWorkspaceLogo from '~/assets/logos/teacher-workspace.svg?react';
import { Badge } from '~/components/ui/badge';
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
  useSidebar,
} from '~/components/ui/sidebar';
import { cn } from '~/helpers/cn';

interface NavItem {
  title: string;
  icon: LucideIcon;
  to: string;
}

const mainNavItems: NavItem[] = [{ title: 'Home', icon: Home, to: '/' }];

const studentInsightsItems: NavItem[] = [{ title: 'Profiles', icon: Users, to: '/students' }];

const communicationsItems: NavItem[] = [
  { title: 'Posts', icon: Mail, to: '/posts' },
  { title: 'Meetings', icon: CalendarClock, to: '/meetings' },
];

const manageItems: NavItem[] = [{ title: 'Student groups', icon: Layers, to: '/groups' }];

// Text that exists only in the expanded rail. Collapsing: fade and blur out in
// 100ms so nothing is caught mid-truncate while the width moves. Expanding:
// resolve back in over 150ms, starting 75ms after the width begins so the
// label lands on a settled column. Opacity and an 8px blur both composite, and
// the blur masks the glyphs re-flowing under the moving edge.
// Reduced motion keeps the 100ms opacity fade and drops the blur and delay.
const RAIL_TEXT_CLASS =
  'tw:transition-[opacity,filter] tw:duration-150 tw:ease-out tw:delay-75 tw:group-data-[collapsible=icon]:opacity-0 tw:group-data-[collapsible=icon]:blur-sm tw:group-data-[collapsible=icon]:duration-100 tw:group-data-[collapsible=icon]:delay-0 tw:motion-reduce:transition-opacity tw:motion-reduce:duration-100 tw:motion-reduce:delay-0 tw:motion-reduce:group-data-[collapsible=icon]:blur-none';

function NavMenuItem({ item }: { item: NavItem }) {
  const { setOpenMobile } = useSidebar();
  const isActive = useMatch({ path: item.to, end: item.to === '/' }) !== null;

  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        render={<Link to={item.to} />}
        isActive={isActive}
        tooltip={item.title}
        onClick={() => setOpenMobile(false)}
      >
        <item.icon className="tw:size-4" />
        <span className={RAIL_TEXT_CLASS}>{item.title}</span>
      </SidebarMenuButton>
    </SidebarMenuItem>
  );
}

function NavMenuItems({ items }: { items: NavItem[] }) {
  return (
    <SidebarMenu>
      {items.map((item) => (
        <NavMenuItem key={item.title} item={item} />
      ))}
    </SidebarMenu>
  );
}

export function AppSidebar() {
  return (
    <SidebarRoot collapsible="icon">
      <SidebarHeader>
        {/* Product identity as one row: the mark, the name and the Beta pill.
            Not a control, so no plate or hover tint. Collapsed, the row
            shrinks to the mark. The mark is centred in the rail by animating
            padding-left, never justify-content: justify-content cannot
            transition, so the mark would teleport on the first frame while the
            width still moves. 2.5rem = the header's 1rem of padding plus the
            1.5rem mark. */}
        <div className="tw:flex tw:h-10 tw:items-center tw:gap-2 tw:pr-0 tw:pl-1 tw:transition-[padding,gap] tw:duration-(--duration-sidebar) tw:ease-in-out tw:select-none tw:group-data-[collapsible=icon]:gap-0 tw:group-data-[collapsible=icon]:pl-[calc((var(--sidebar-width-icon)-2.5rem)/2)] tw:motion-reduce:transition-none">
          <TeacherWorkspaceLogo aria-hidden className="tw:size-6 tw:shrink-0" />
          <span
            className={cn(
              'tw:min-w-0 tw:flex-1 tw:truncate tw:text-[13px] tw:font-semibold tw:text-sidebar-foreground tw:group-data-[collapsible=icon]:flex-[0]',
              RAIL_TEXT_CLASS,
            )}
          >
            Teacher Workspace
          </span>
          <Badge
            variant="info"
            className={cn(
              'tw:group-data-[collapsible=icon]:w-0 tw:group-data-[collapsible=icon]:border-0 tw:group-data-[collapsible=icon]:px-0',
              RAIL_TEXT_CLASS,
            )}
          >
            Beta
          </Badge>
        </div>
      </SidebarHeader>

      <SidebarContent className="tw:gap-0">
        <SidebarGroup>
          <SidebarGroupContent>
            <NavMenuItems items={mainNavItems} />
          </SidebarGroupContent>
        </SidebarGroup>
        <SidebarGroup>
          <SidebarGroupLabel>Student Insights</SidebarGroupLabel>
          <SidebarGroupContent>
            <NavMenuItems items={studentInsightsItems} />
          </SidebarGroupContent>
        </SidebarGroup>
        <SidebarGroup>
          <SidebarGroupLabel>Communications</SidebarGroupLabel>
          <SidebarGroupContent>
            <NavMenuItems items={communicationsItems} />
          </SidebarGroupContent>
        </SidebarGroup>
        <SidebarGroup>
          <SidebarGroupLabel>Manage</SidebarGroupLabel>
          <SidebarGroupContent>
            <NavMenuItems items={manageItems} />
          </SidebarGroupContent>
        </SidebarGroup>
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
              <CircleHelp className="tw:size-4" />
              <span className={RAIL_TEXT_CLASS}>Help</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
    </SidebarRoot>
  );
}
