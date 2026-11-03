import { createApp } from './app.ts';
import { loadConfig } from './config.ts';
import { createProvider } from './provider.ts';

const config = loadConfig(process.env);
const app = createApp(createProvider(config));

const server = app.listen(config.port, () => {
  // oxlint-disable-next-line no-console
  console.log(`mock-edupass listening on port ${config.port} as ${config.url}`);
});

for (const signal of ['SIGTERM', 'SIGINT'] as const) {
  process.once(signal, () => {
    server.close(() => process.exit(0));
    server.closeIdleConnections();
  });
}
