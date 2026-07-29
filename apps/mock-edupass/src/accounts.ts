import type { Account, AccountClaims, KoaContextWithOIDC } from 'oidc-provider';

/**
 * FakeAccount is a hard-coded stand-in for an Edupass teacher account. There is no
 * password: the account is selected from a list, so `sub` is the only identity that
 * matters. An optional field left unset is omitted from the ID token entirely.
 */
export interface FakeAccount {
  /** Stable subject identifier, emitted as the `sub` claim. */
  sub: string;
  /** Email address, emitted as the `email` claim. */
  email: string;
  /** Display name, emitted as the `name` claim. Omitted when unset. */
  name?: string;
}

/**
 * FAKE_ACCOUNTS contains every account the provider will authenticate. It is the whole
 * account store: there is no database, config file, or environment variable behind it.
 */
export const FAKE_ACCOUNTS: readonly FakeAccount[] = [
  { sub: 'teacher-001', email: 'amirah.rahman@schools.gov.sg', name: 'Amirah Rahman' },
  { sub: 'teacher-002', email: 'wei.ling.tan@schools.gov.sg', name: 'Tan Wei Ling' },
  // Deliberately nameless. Edupass may omit claims, and a relying party that assumes
  // every claim is present should fail here rather than in front of a teacher.
  { sub: 'teacher-003', email: 'no.name@schools.gov.sg' },
];

/** findFakeAccount returns the account with the given subject identifier, or undefined. */
export function findFakeAccount(sub: string): FakeAccount | undefined {
  return FAKE_ACCOUNTS.find((account) => account.sub === sub);
}

/**
 * toClaims builds the claim set for an account. Keys the account leaves unset are absent
 * rather than null or empty, so a relying party can distinguish "not provided" from "blank".
 */
export function toClaims(account: FakeAccount): AccountClaims {
  const claims: AccountClaims = { sub: account.sub, email: account.email };
  if (account.name !== undefined) {
    claims.name = account.name;
  }
  return claims;
}

/** findAccount resolves a subject identifier for oidc-provider, or undefined if unknown. */
export function findAccount(_ctx: KoaContextWithOIDC, id: string): Account | undefined {
  const account = findFakeAccount(id);
  if (account === undefined) {
    return undefined;
  }
  return { accountId: account.sub, claims: () => toClaims(account) };
}
