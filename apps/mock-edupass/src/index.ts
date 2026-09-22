import { createApp } from './app.ts';
import { config } from './config.ts';
import { clientJwk } from './jwks.ts';

try {
  const { app } = createApp(config.port, clientJwk(config.clientPublicKey));

  app.listen(config.port, () => {
    // oxlint-disable-next-line no-console
    console.log(`mock-edupass listening on http://localhost:${config.port}`);
  });
} catch (err) {
  // oxlint-disable-next-line no-console
  console.error(err instanceof Error ? err.message : err);
  process.exit(1);
}
