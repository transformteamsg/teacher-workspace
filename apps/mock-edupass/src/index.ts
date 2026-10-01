import { createApp } from './app.ts';
import { loadConfig } from './config.ts';
import { createProvider } from './provider.ts';

const config = loadConfig(process.env);
const app = createApp(createProvider(config));

app.listen(config.port, () => {
  // oxlint-disable-next-line no-console
  console.log(`mock-edupass listening on http://localhost:${config.port}`);
});
