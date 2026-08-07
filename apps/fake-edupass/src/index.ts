import express from 'express';

import { config } from './config.ts';
import { provider } from './provider.ts';

const app = express();

app.get('/health', (_req, res) => {
  res.sendStatus(200);
});

app.use(provider.callback());

app.listen(config.port, () => {
  // eslint-disable-next-line no-console
  console.log(`fake-edupass listening on http://localhost:${config.port}`);
});
