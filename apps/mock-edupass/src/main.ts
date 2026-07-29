import { startProvider } from './server.ts';

/** DEFAULT_PORT avoids 3000 (the Go server) and 3001 (the Rsbuild dev server). */
const DEFAULT_PORT = 3002;

function resolvePort(value: string | undefined): number {
  if (value === undefined) {
    return DEFAULT_PORT;
  }

  const port = Number(value);
  if (!Number.isInteger(port) || port < 0 || port > 65535) {
    throw new Error(`want PORT: an integer between 0 and 65535; got: ${value}`);
  }
  return port;
}

const { url } = await startProvider(resolvePort(process.env.PORT));

// Telling the developer where it is listening is the entry point's job, so this is the one
// place in the package where writing to stdout is the intended behaviour.
// eslint-disable-next-line eslint/no-console
console.log(
  `mock-edupass listening on ${url}\ndiscovery: ${url}/.well-known/openid-configuration`,
);
