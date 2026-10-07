import { registerRemotes } from '@module-federation/enhanced/runtime';
import React from 'react';
import { createRoot } from 'react-dom/client';

import { preloadedState } from '~/stores/preloaded-state';

import './App.css';
import App from './App';

// Register the remotes immediately so that remote routes can load their modules.
registerRemotes([...preloadedState.remotes]);

const container = document.getElementById('root');
if (!container) throw new Error('Page has no #root element');

createRoot(container).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
