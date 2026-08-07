import express from 'express';

import { createInteractionRouter } from './interaction.ts';
import { createProvider } from './provider.ts';

export function createApp(port: number) {
  const provider = createProvider(port);
  const interactionRouter = createInteractionRouter(provider);

  const app = express();

  app.get('/health', (_req, res) => {
    res.sendStatus(200);
  });

  app.use(interactionRouter);
  app.use(provider.callback());

  return { app, provider };
}
