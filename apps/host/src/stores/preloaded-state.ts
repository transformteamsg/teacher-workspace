/**
 * The state the server embeds in the page, read once at startup.
 */
export interface PreloadedState {
  csrfToken: string;
  remotes: readonly Remote[];
  user?: User;
}

/**
 * A Module Federation remote the host registers at startup.
 */
export interface Remote {
  name: string;
  entry: string;
}

/**
 * The signed-in teacher, absent when the session is unauthenticated. `role`
 * is their resolved role's display label, such as "Teacher", and `roleCode`
 * its Edupass code, such as "ROLE_TEACHER". `attributes` holds their Edupass
 * attribute codes, such as "ATTR_PG_ADMIN", and is empty when they have none.
 */
export interface User {
  name: string;
  email: string;
  role: string;
  roleCode: string;
  attributes: readonly string[];
}

const INVALID_PRELOADED_STATE = 'Page has no valid preloaded state';

/**
 * Reads the state the server embedded in the page.
 *
 * @throws {Error} When the page has no preloaded state, or it does not match {@link PreloadedState}.
 */
function readPreloadedState(): PreloadedState {
  const source = document.getElementById('preloaded-state')?.textContent ?? '';

  let state: unknown;
  try {
    state = JSON.parse(source);
  } catch (err) {
    throw new Error(INVALID_PRELOADED_STATE, { cause: err });
  }

  if (!isPreloadedState(state)) {
    throw new Error(INVALID_PRELOADED_STATE);
  }
  return state;
}

function isPreloadedState(value: unknown): value is PreloadedState {
  return (
    typeof value === 'object' &&
    value !== null &&
    typeof (value as Partial<PreloadedState>).csrfToken === 'string' &&
    Array.isArray((value as Partial<PreloadedState>).remotes) &&
    (value as PreloadedState).remotes.every(isRemote) &&
    ((value as Partial<PreloadedState>).user === undefined ||
      isUser((value as PreloadedState).user))
  );
}

function isUser(value: unknown): value is User {
  return (
    typeof value === 'object' &&
    value !== null &&
    typeof (value as Partial<User>).name === 'string' &&
    typeof (value as Partial<User>).email === 'string' &&
    typeof (value as Partial<User>).role === 'string' &&
    typeof (value as Partial<User>).roleCode === 'string' &&
    Array.isArray((value as Partial<User>).attributes) &&
    (value as User).attributes.every((attribute) => typeof attribute === 'string')
  );
}

function isRemote(value: unknown): value is Remote {
  return (
    typeof value === 'object' &&
    value !== null &&
    typeof (value as Partial<Remote>).name === 'string' &&
    typeof (value as Partial<Remote>).entry === 'string'
  );
}

export const preloadedState: Readonly<PreloadedState> = readPreloadedState();
