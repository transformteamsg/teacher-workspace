import http from 'node:http';

import type Provider from 'oidc-provider';

import { createApp } from './app.ts';
import { createProvider } from './provider.ts';

/** RunningProvider is a started provider and the handle to shut it down. */
export interface RunningProvider {
  /** url is the issuer the provider advertises and signs tokens with. */
  url: string;
  /** address is the loopback origin the socket is bound to, which url may differ from. */
  address: string;
  /** close stops the server and resolves once the port is released. */
  close: () => Promise<void>;
}

export interface StartProviderOptions {
  /** TCP port to bind. 0, the default, takes an ephemeral port. */
  port?: number;
  /**
   * Issuer advertised in discovery and carried as the `iss` claim. Defaults to the bound
   * loopback address.
   *
   * Set it whenever the provider is reached under another name, such as `localhost` or a
   * container service name. OIDC Discovery requires the advertised issuer to be identical
   * to the URL the document was fetched from, and the library derives the other endpoint
   * URLs from the request Host header, so a mismatch leaves the document contradicting
   * itself and a conforming relying party refusing it.
   */
  issuer?: string;
  /**
   * buildHandler builds the request listener from the provider. Defaults to the correctly
   * wired application, and exists so tests can mount a deliberately different arrangement
   * without duplicating the socket lifecycle below.
   */
  buildHandler?: (provider: Provider) => http.RequestListener;
}

/**
 * startProvider binds a socket, then builds the provider around the address it got. The
 * order matters: discovery advertises absolute URLs, so the issuer cannot be derived until
 * the port is known.
 */
export async function startProvider(options: StartProviderOptions = {}): Promise<RunningProvider> {
  const { port = 0, issuer, buildHandler = createApp } = options;
  const server = http.createServer();

  await new Promise<void>((resolve, reject) => {
    server.once('error', reject);
    server.listen(port, '127.0.0.1', () => {
      server.removeListener('error', reject);
      resolve();
    });
  });

  // The listen listener above is removed once binding succeeds, so without a replacement a
  // later error event would surface as an uncaught exception and take the process down.
  server.on('error', (err) => {
    // eslint-disable-next-line eslint/no-console
    console.error('mock-edupass server error:', err);
  });

  const address = server.address();
  if (address === null || typeof address === 'string') {
    throw new Error(`want: a bound TCP address; got: ${String(address)}`);
  }

  const origin = `http://127.0.0.1:${address.port}`;
  const url = issuer ?? origin;
  server.on('request', buildHandler(createProvider(url)));

  return {
    url,
    address: origin,
    close: () =>
      new Promise<void>((resolve, reject) => {
        server.close((err) => (err === undefined ? resolve() : reject(err)));
      }),
  };
}
