import AllEars from '~/assets/logos/allears.svg?react';
import Allocate from '~/assets/logos/allocate.svg?react';
import Appraiser from '~/assets/logos/appraiser.svg?react';
import Connectogram from '~/assets/logos/connectogram.svg?react';
import Glow from '~/assets/logos/glow.svg?react';
import Hrp from '~/assets/logos/hrp.svg?react';
import Langbuddy from '~/assets/logos/langbuddy.svg?react';
import Mysei from '~/assets/logos/mysei.svg?react';
import Opal from '~/assets/logos/opal.svg?react';
import SchoolCockpit from '~/assets/logos/schoolcockpit.svg?react';
import ScMobile from '~/assets/logos/scmobile.svg?react';
import Sdis from '~/assets/logos/sdis.svg?react';
import Sls from '~/assets/logos/sls.svg?react';
import Students from '~/assets/logos/students.svg?react';
import Workpal from '~/assets/logos/workpal.svg?react';
import { type App, FeaturedAppCard } from '~/components/AppCard';
import { AppSection, type AppSectionProps } from '~/components/AppSection';

const FEATURED_APP: App = {
  name: 'Student Insights',
  description: 'Holistic insights that help every student thrive',
  icon: Students,
  href: '/students',
  badge: 'Beta',
};

const APP_SECTIONS: AppSectionProps[] = [
  {
    title: 'Frequently Used',
    apps: [
      {
        name: 'School Cockpit',
        description: 'Your central hub for school management and daily operations',
        icon: SchoolCockpit,
        href: 'https://schoolcockpit.moe.gov.sg',
      },
      {
        name: 'SC Mobile',
        description: 'Streamlined attendance management on the go',
        icon: ScMobile,
        href: 'https://scmobile.moe.edu.sg/login',
      },
      {
        name: 'SLS',
        description: 'Teaching and learning platform for curriculum aligned resources',
        icon: Sls,
        href: 'https://vle.learning.moe.edu.sg/login',
      },
    ],
  },
  {
    title: 'Student Information',
    apps: [
      {
        name: 'All Ears',
        description: 'Personalised forms for students, staff, and parents',
        icon: AllEars,
        href: 'https://forms.moe.edu.sg',
      },
      {
        name: 'Student Insights',
        description: 'Holistic insights that help every student thrive',
        icon: Students,
        href: '/students',
      },
      {
        name: 'Allocate',
        description: 'Simplify your Full SBB class allocation',
        icon: Allocate,
        href: 'https://allocate.digital.moe.gov.sg',
      },
      {
        name: 'SDIS',
        description:
          'One-stop platform for National School Games, Singapore Youth Festival, Outdoor Adventure Learning Centre, and MOE OBS Challenge.',
        icon: Sdis,
        href: 'https://www.sdis.moe.gov.sg/oalc/s/login',
      },
    ],
  },
  {
    title: 'Social-Emotional & Mental Wellbeing (SEConnect)',
    apps: [
      {
        name: 'MySEI',
        description: 'Holistic insights for students’ social-emotional growth & wellbeing',
        icon: Mysei,
        href: 'https://mysei.digital.moe.gov.sg',
      },
      {
        name: 'Connecto-gram',
        description: 'Social network analysis for student connectedness and peer relationships',
        icon: Connectogram,
        href: 'https://forms.moe.edu.sg/sna/manage/forms',
      },
      {
        name: 'Termly Check-In',
        description: 'Regular wellbeing check-ins to support student mental health',
        icon: AllEars,
        href: 'https://forms.moe.edu.sg/dashboards',
      },
    ],
  },
  {
    title: 'AI Productivity Tools',
    apps: [
      {
        name: 'Appraiser',
        description: 'AI-generated draft student testimonials in seconds',
        icon: Appraiser,
        href: 'https://smartcompose.gov.sg',
      },
    ],
  },
  {
    title: 'Teaching & Learning',
    apps: [
      {
        name: 'LangBuddy',
        description:
          'AI conversational chatbot for Mother Tongue Language learning for Secondary Schools',
        icon: Langbuddy,
        href: 'https://langbuddy.moe.edu.sg',
      },
    ],
  },
  {
    title: 'Admin',
    apps: [
      {
        name: 'Workpal',
        description: 'Your workplace management companion',
        icon: Workpal,
        href: 'https://app.workpal.gov.sg',
      },
      {
        name: 'HR and Payroll Portal (HRP)',
        description: 'One-stop platform for staff to manage leave, claims, and HR admin tasks',
        icon: Hrp,
        href: 'https://www.hrp.gov.sg',
      },
    ],
  },
  {
    title: 'Professional Development',
    apps: [
      {
        name: 'OPAL 2.0',
        description: 'One-stop portal for professional learning',
        icon: Opal,
        href: 'https://idm.opal2.moe.edu.sg',
      },
      {
        name: 'Glow',
        description: 'Bite-sized daily learning in just 5 minutes',
        icon: Glow,
        href: 'https://glow.digital.moe.gov.sg/home',
      },
    ],
  },
];

function getGreeting(): string {
  const hour = new Date().getHours();
  if (hour < 12) return 'Good morning';
  if (hour < 17) return 'Good afternoon';
  return 'Good evening';
}

export default function HomeView() {
  return (
    <main className="tw:mx-auto tw:flex tw:max-w-page tw:flex-col tw:gap-8 tw:px-4 tw:py-4.5">
      <h1 className="tw:py-0 tw:text-center tw:text-2xl tw:font-semibold tw:text-foreground">
        {getGreeting()}
      </h1>

      <section className="tw:flex tw:flex-col tw:gap-4">
        <h2 className="tw:text-lg tw:font-semibold tw:text-foreground">Featured</h2>
        <FeaturedAppCard {...FEATURED_APP} />
      </section>

      {APP_SECTIONS.map((section) => (
        <AppSection key={section.title} {...section} />
      ))}
    </main>
  );
}
