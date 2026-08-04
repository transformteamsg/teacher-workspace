import { startProvider } from './server.ts';

/** DEFAULT_PORT avoids 3000 (the Go server) and 3001 (the Rsbuild dev server). */
const DEFAULT_PORT = 3002;

/**
 * resolvePort reads a port from the environment. An unset or blank value falls back to the
 * default, which is what an undefined shell or compose variable expands to, and anything that
 * is not plain decimal digits is rejected rather than coerced: `Number` accepts `0x10`, `1e3`,
 * and the empty string, so relying on it would silently bind a port nobody asked for.
 */
function resolvePort(value: string | undefined): number {
  const trimmed = value?.trim() ?? '';
  if (trimmed === '') {
    return DEFAULT_PORT;
  }

  if (!/^\d+$/.test(trimmed)) {
    throw new Error(`want PORT: decimal digits only; got: ${JSON.stringify(value)}`);
  }

  const port = Number(trimmed);
  if (port > 65535) {
    throw new Error(`want PORT: at most 65535; got: ${trimmed}`);
  }
  return port;
}

/**
 * resolveIssuer reads the advertised issuer. Leave it unset for local use and the provider
 * uses its bound loopback address. Set it when the provider is reached under another name,
 * such as `localhost` or a container service name, so discovery stays self-consistent.
 */
function resolveIssuer(value: string | undefined): string | undefined {
  const trimmed = value?.trim() ?? '';
  if (trimmed === '') {
    return undefined;
  }

  let parsed: URL;
  try {
    parsed = new URL(trimmed);
  } catch {
    throw new Error(`want MOCK_EDUPASS_ISSUER: an absolute URL; got: ${JSON.stringify(value)}`);
  }

  if (parsed.search !== '' || parsed.hash !== '') {
    throw new Error(`want MOCK_EDUPASS_ISSUER: no query or fragment; got: ${trimmed}`);
  }
  return trimmed.replace(/\/$/, '');
}

const { url, address } = await startProvider({
  port: resolvePort(process.env.PORT),
  issuer: resolveIssuer(process.env.MOCK_EDUPASS_ISSUER),
});

// Telling the developer where it is listening is the entry point's job, so this is the one
// place in the package where writing to stdout is the intended behaviour. The bound address is
// reported separately because a configured issuer does not have to resolve to it.
const lines = [`mock-edupass listening on ${address}`];
if (url !== address) {
  lines.push(`issuer: ${url}`);
}
lines.push(`discovery: ${address}/.well-known/openid-configuration`);

// eslint-disable-next-line eslint/no-console
console.log(lines.join('\n'));
