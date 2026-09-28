import { resolve } from 'node:path';

import { ensureKeyPair } from '../lib/certificate.ts';

const keyDir = resolve(import.meta.dirname, '../../..', '.certs');
await ensureKeyPair(keyDir);

await import('../src/index.ts');
