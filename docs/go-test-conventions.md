# Go Test Conventions

Tests aim for clear, actionable failure messages and isolation from implementation details.

## Structure

- One parent test per function or method under test, named `Test<Func>` or `Test<Type>_<Method>`, with every case as a `t.Run` subtest. Small pure helpers can have standalone tests without subtests.
- One behaviour, one subtest. A behaviour is one rule of the unit's contract applied to one scenario (a starting state plus an action), with every outcome the rule promises for it.
  - Keep a rule's outcomes in one subtest. If any of them breaks, the rule is broken, so splitting them only repeats the setup.
  - Give each independent rule its own subtest, even when they share a scenario. Each can break without the others, so a separate subtest keeps its name short and makes a failure point to the rule that broke.
- One code path, one subtest. If two subtests send the unit down the same path and differ only in why a dependency failed, keep one: the dependency's own tests cover its failure modes.
- Use table-driven tests only when cases differ purely in their inputs. A branch in setup or assertions means separate subtests directly under the parent test.
- Order subtests by the unit's lifecycle (for example, load before save), not by outcome.

## Naming

- Name a subtest for its behaviour: the rule it checks and the scenario, in a short sentence. Don't list the rule's outcomes: the assertions check them, and their failure messages name what broke. The subtest asserts nothing outside that rule, apart from guards that make the assertions safe or meaningful (a nil check before use, a check that the scenario actually happened).
- Use one verb that covers every outcome the subtest asserts: `"replaces the handler's response with a 500 when the session cannot be saved"` covers the status, headers, and body. If no verb covers them all, the subtest checks more than one rule, so split it.
  - Avoid a verb another subtest contradicts: `"passes the handler's response through"`, when the middleware also rewrites `Cache-Control`.
- Use the contract's words from its godoc ("first write", "cannot be saved"), and describe the scenario at the unit's level, not a dependency's internals.
- Name a table-driven subtest for what every case checks, and each case for what differs, so the two read as one sentence. Don't repeat words such as "when" in both.
  - `"sets the session cookie's Secure attribute"` / `"to true when the Secure option is set"`
- Word paired subtests (two sides of one rule) the same way, so their names differ only where the scenarios do.
  - `"saves changes made before the handler's first write"` / `"does not save changes made after the handler's first write"`

## Isolation

Test a unit the way its callers use it. Such tests survive refactoring and fail only when a caller would notice.

- A unit is a function, method, or type with a contract stated in its doc comment, exported or not. A function split out only to keep another one readable, with no callers or contract of its own, is an implementation detail of that function: test it through that function, so inlining or renaming it doesn't break tests.
- Set up and check state through APIs, the unit's own included, even when one type provides both sides: store with `Set` and read back with `Get`, not through the internal map. A bug in `Get` then also fails `Set`'s tests, but `Get`'s own tests fail alongside and point to it.
- Reach into unexported fields only when no API can set up the scenario or show the outcome, such as seeding a corrupted entry that `Set` would never write, or checking that an expired entry was freed. Don't export a field or add a method just so a test can reach it.
- When other programs read the data a unit writes, check the data where they read it. If other services read a cache's keys straight from Valkey, check the key in Valkey after `Set`, not only through `Get`: if `Set` and `Get` both used the wrong key, `Get` would still find the value.

## Setup

- Prefer `t.Setenv`, `t.TempDir`, and `t.Chdir` to manual save/restore. They restore the previous state automatically, including on failure.
- Register `t.Cleanup` immediately after the state is mutated, not further down the test body.
- A failing setup call is not an assertion: `t.Fatalf("os.WriteFile: %v", err)`, naming the operation, never a `want`/`got` message. Use `t.Fatal`, not `t.Error`: the case can't run.
- Arrange the starting state so a broken unit can't pass by accident: seed any state the test claims is kept, so an overwrite or delete shows, and use values that differ from the defaults (a handler status of 201, not the recorder's default 200).

## Assertions

- When a call returns something, assert on what it returns.
- When a call changes something, assert on its direct effects: the response, what a handler receives, what a store holds, what a later call returns.
- Don't assert on the calls the unit makes to its dependencies, except in two cases:
  - The call is the behaviour (`"sends one email"`): count it.
  - The effect only shows once the dependency acts on it, which the dependency's own tests cover: check what was passed instead, such as the TTL given to a cache, not whether the entry expires after a fake clock moves past it.
- Stop the subtest with a `t.Fatal` guard when the checks after it would panic or prove nothing:
  - Before using a value: `if user == nil { t.Fatal("want user: non-nil; got: nil") }`
  - Before indexing: `if len(items) == 0 { t.Fatal("want items: non-empty; got: empty") }`. Use an exact count (`len(items) != 1`) only in the subtest whose behaviour it is, so only that subtest fails when the count changes.
  - Before comparing a looked-up value (`value, ok := m[k]`): check `ok` in its own `if`, then compare the value. In a handler or callback, skipping this lets a nil dereference panic and stop the whole parent test.
  - Before checking the outcome, confirm the scenario happened, for example that a rotated token changed: `if oldToken == newToken { t.Fatalf("want token: != %q; got: %q", oldToken, newToken) }`
- When you add or change an assertion, temporarily break the code it targets and confirm it fails.
- Use `want`/`got` style, with `want` on the left (`!=` for equality, `==` for change from baseline). Failure messages read `want <subject>: <expected>; got: <actual>`, so a failure is clear without opening the test. The subject is the checked expression as written (`user.IsActive()`, `cookie.MaxAge`), or a short name for it when the expression is mostly plumbing (`Set-Cookie` for `rec.Result().Header.Values("Set-Cookie")`). Errors use `err` as their subject, and lookups name what was looked up before `ok` (`cookies["theme"] ok`).
- Call the unit under test in its own statement and assign what it returns to variables named for what they hold (`chained := Chain(h)`), so the action stands apart from the assertions that check it. Don't call it inside an assertion's `if` initialiser: `if got := Chain(h); h != got` hides the action inside the check.
- Bind a value to `want` or `got` in the `if` initialiser when the failure message prints it, so the message shows exactly what the condition tested. A plain variable ("named") needs no binding and prints as it is; a field, lookup, literal, expression, or call other than to the unit under test ("fresh") gets bound. Give setup variables semantic names so `want` and `got` stay free.
  - both named: `if a != b { ... }`
  - one fresh: `if got := <fresh>; a != got { ... }` (bind `want :=` instead when the fresh side is the expected value)
  - both fresh: `if want, got := X, Y; want != got { ... }`
  - one side, named: `if err != nil { ... }`
  - one side, fresh, printed: `if got := <fresh>; len(got) != 0 { ... }`
  - one side, fresh, not printed: `if len(items) == 0 { ... }`, `if !user.IsActive() { ... }`

### Failure message templates

- Equality: `t.Errorf("want status: %d; got: %d", want, got)`
- Change from baseline (fails on `want == got`): `t.Errorf("want token: != %q; got: %q", want, got)`
- Property: `t.Errorf("want Set-Cookie: empty; got: %q", got)` (descriptors: `nil`, `non-nil`, `empty`, `non-empty`)
- Nil: `t.Error("want result: nil; got: non-nil")` / `t.Fatal("want user: non-nil; got: nil")`. A nil check reports a descriptor on both sides, not the value.
- Boolean: `t.Error("want user.IsActive(): true; got: false")`
- Error (nil): `t.Fatalf("want err: nil; got: %v", err)`
- Error (sentinel): `t.Errorf("want err: %v; got: %v", ErrInvalidInput, err)`
- Presence: `t.Fatal("want cookies[\"theme\"] ok: true; got: false")`
- Containment (generic): `t.Errorf("want err: containing %q; got: %q", "timeout", err)`
- Containment (constant): `t.Errorf("want c: in AlphabetBase58; got: %q", c)`
- Panic: `t.Fatal("want MustParse(\"\"): panic; got: nil")` / `t.Errorf("want MustParse(\"x\"): no panic; got: %v", r)`

## Logs

- Treat logging as its own rule: check log output only in a subtest named for it (`"logs an error when the session cannot be saved"`), or one whose name promises no effects at all (`"does nothing when …"`).
- Capture logs with a `slog.JSONHandler` on a `bytes.Buffer`, attached the way the code reads its logger (for example `middleware.WithLogger`). Check that only the expected records were written, each with its level, message, and attributes (for an error log, an `err` containing the cause).
- Attach `slog.New(slog.DiscardHandler)` in every subtest of a unit that can log but doesn't check log output, even when the scenario logs nothing, so a stray log can't clutter the output or panic on a nil logger. A unit with no logger to reach, such as a context accessor, needs none.

## Helpers

- Keep the scenario in the subtest body: the setup that varies, the action, and the assertions that state the behaviour. Extract only mechanics that are identical everywhere, carry no subtest-specific meaning, and are named for what they check or build, so the call can be trusted without opening them. Wait until enough subtests repeat it to show what's truly shared.
- Name a test double for what it does, followed by what it replaces, not for its kind: `failingWriter`, `fixedClock`, `recordingHandler`. A package's one general-purpose, configurable stand-in for an interface can take a generic name instead (`fakeStore`).
- Put helpers, including test doubles and their types, at the bottom of the test file that uses them, so readers meet the tests first. When several test files in the package need one, move it to its own test file (`fakestore_test.go`).
