import { createApp } from './app.ts';
import { config } from './config.ts';

const { app } = createApp(config.port);

app.listen(config.port, () => {
  // eslint-disable-next-line no-console
  console.log(`mock-edupass listening on http://localhost:${config.port}`);
});
