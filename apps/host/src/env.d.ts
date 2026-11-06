/// <reference types="@rsbuild/core/types" />

declare module '*.svg?react' {
  const ReactComponent: React.FunctionComponent<React.SVGProps<SVGSVGElement>>;
  export default ReactComponent;
}

interface ImportMetaEnv {
  /** `'true'` hides Posts and Student groups. TODO(#306): remove. */
  readonly PUBLIC_HIDE_PG_FEATURES?: string;
}
