import http from 'node:http';

import { createApp } from './app.ts';
import { createProvider } from './provider.ts';

/** RunningProvider is a started provider and the handle to shut it down. */
export interface RunningProvider {
  /** url is the issuer, which always matches the address the socket actually bound. */
  url: string;
  /** close stops the server and resolves once the port is released. */
  close: () => Promise<void>;
}

/**
 * startProvider binds a socket, then builds the provider around the address it got. The
 * order matters: discovery advertises absolute URLs, so the issuer cannot be known until
 * the port is. Pass 0 (the default) for an ephemeral port, which lets tests run in parallel.
 */
export async function startProvider(port = 0): Promise<RunningProvider> {
  const server = http.createServer();

  await new Promise<void>((resolve, reject) => {
    server.once('error', reject);
    server.listen(port, '127.0.0.1', () => {
      server.removeListener('error', reject);
      resolve();
    });
  });

  const address = server.address();
  if (address === null || typeof address === 'string') {
    throw new Error(`want: a bound TCP address; got: ${String(address)}`);
  }

  const url = `http://127.0.0.1:${address.port}`;
  server.on('request', createApp(createProvider(url)));

  return {
    url,
    close: () =>
      new Promise<void>((resolve, reject) => {
        server.close((err) => (err === undefined ? resolve() : reject(err)));
      }),
  };
}
