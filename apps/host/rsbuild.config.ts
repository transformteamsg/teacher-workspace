import { pluginModuleFederation } from '@module-federation/rsbuild-plugin';
import { defineConfig } from '@rsbuild/core';
import { pluginReact } from '@rsbuild/plugin-react';
import { pluginTailwindcss } from '@rsbuild/plugin-tailwindcss';

export default defineConfig({
  plugins: [
    pluginReact(),
    pluginTailwindcss(),
    pluginModuleFederation({
      name: 'teacher_workspace',
      remotes: {},
      shared: {
        react: {
          singleton: true,
          requiredVersion: '^19.2.7',
        },
        'react-dom': {
          singleton: true,
          requiredVersion: '^19.2.7',
        },
        'react-router': {
          singleton: true,
          requiredVersion: '^8.2.0',
        },
      },
    }),
  ],
  html: {
    template: './index.html',
  },
  server: {
    host: '127.0.0.1',
    port: 3001,
  },
});
