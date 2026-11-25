import assert from 'node:assert/strict';
import { afterEach, describe, it } from 'node:test';

type Store = typeof import('../src/stores/preloaded-state.ts');

const user = {
  name: 'Tan Mei Ling',
  email: 'tan_mei_ling@schools.gov.sg',
  role: 'Teacher',
  roleCode: 'ROLE_TEACHER',
  attributes: ['ATTR_PG_USER'],
};

let imports = 0;

// The store reads the page once, as it is imported, so each call imports a
// fresh copy of it against a page that embeds `state`.
function importStore(state: unknown): Promise<Store> {
  globalThis.document = {
    getElementById: (id: string) =>
      id === 'preloaded-state' ? { textContent: JSON.stringify(state) } : null,
  } as unknown as Document;

  return import(`../src/stores/preloaded-state.ts?import=${imports++}`);
}

afterEach(() => {
  Reflect.deleteProperty(globalThis, 'document');
});

describe('preloadedState', () => {
  describe('holds the user the page embeds', () => {
    const tests = [
      { name: 'when they have attributes', user },
      { name: 'when they have no attributes', user: { ...user, attributes: [] } },
    ];

    for (const test of tests) {
      it(test.name, async () => {
        const { preloadedState } = await importStore({
          csrfToken: 'token',
          remotes: [],
          apps: [],
          user: test.user,
        });

        assert.deepEqual(preloadedState.user, test.user);
      });
    }
  });

  describe('throws when the page embeds a user', () => {
    const tests = [
      // JSON leaves out an undefined field, so the page embeds no roleCode.
      { name: 'with no roleCode', user: { ...user, roleCode: undefined } },
      { name: 'whose attributes are not a list', user: { ...user, attributes: null } },
      {
        name: 'whose attributes hold a value that is not a string',
        user: { ...user, attributes: ['ATTR_PG_USER', 1] },
      },
    ];

    for (const test of tests) {
      it(test.name, async () => {
        await assert.rejects(
          importStore({ csrfToken: 'token', remotes: [], apps: [], user: test.user }),
          /Page has no valid preloaded state/,
        );
      });
    }
  });
});
