import { registerRemotes } from '@module-federation/enhanced/runtime';
import React from 'react';
import { createRoot } from 'react-dom/client';

import './App.css';
import App from './App';

interface RuntimeRemote {
  name: string;
  entry: string;
}

interface RuntimeConfig {
  remotes: RuntimeRemote[];
}

// A failed fetch leaves the shell running with no remotes: the routes that need
// one show their fallback instead of the whole page going blank.
async function loadRuntimeConfig(): Promise<RuntimeConfig> {
  try {
    const response = await fetch('/config.json');
    if (!response.ok) {
      throw new Error(`/config.json responded ${response.status}`);
    }
    const config = (await response.json()) as Partial<RuntimeConfig>;
    return { remotes: config.remotes ?? [] };
  } catch (error: unknown) {
    console.error('Could not load /config.json; no remotes registered', error);
    return { remotes: [] };
  }
}

async function boot() {
  const container = document.getElementById('root');
  if (!container) throw new Error('Root element #root not found');

  // Registered once, before the first render, so a route that lazy-loads a
  // remote never races the fetch that decides where that remote lives.
  const { remotes } = await loadRuntimeConfig();
  if (remotes.length > 0) {
    registerRemotes(remotes);
  }

  createRoot(container).render(
    <React.StrictMode>
      <App />
    </React.StrictMode>,
  );
}

boot().catch((error: unknown) => {
  console.error('Failed to start the app', error);
});
