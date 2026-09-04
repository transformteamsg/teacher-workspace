# 0001 Local Development Environment for Remote Applications

**Author(s):**

- Goh Jun Hong / [@junnhooong](https://github.com/junnhooong)

**Status:** Accepted

## Context

Teacher Workspace is a Module Federation host shell. The applications teachers actually use are **remote applications (MFEs)**, and each one lives in its own repository, owned by its own team. Nothing in this repository is checked out on a remote app developer's machine.

That leaves those developers without a way to exercise the full flow. Running their own dev server on its own only renders the MFE in isolation: no host shell around it, no session, and no reachable back end. To see what they are actually shipping, they need the parts of the flow that live outside their repository, and in production that flow has four hops, only the last of which belongs to the remote app team:

1. The browser loads the **Teacher Workspace host shell**, which owns the cookie-based session, the surrounding navigation and layout, and the routing that mounts the remote.
2. The host mounts the **remote app** from its remote entry. The remote renders inside the shell and inherits the session from it.
3. Every request the remote makes goes to the **Teacher Workspace back-end proxy** on the same origin, carrying the session cookie. The proxy authenticates the session, mints the JWT for the caller, and forwards the request onward.
4. The **remote app's own back end** receives the forwarded request, already carrying a JWT, and answers it.

Three approaches were tabled and their trade-offs reviewed:

1. **Container image bundling the host shell and the proxy.** Developers run one image locally provided by Teacher Workspace, pointed at their remote entry and their local back end. It bundles everything the host side of the flow needs to stand up on its own: the Teacher Workspace front end, the Teacher Workspace back end, the database, and Valkey. Two environment variables wire in the developer's side — one for the remote entry served by their local front-end dev server, one for the base URL of their local back end. It serves the host shell, issues a mock session, performs the cookie-to-JWT translation, and forwards to the developer's back end, reproducing all four hops on localhost. Costs are image size and the need for an update/versioning story to prevent drift from the real host and proxy.
2. **Hosted / public proxy (API Gateway).** Developers point their local remote at a shared proxy and authenticate with an API key. Always current with the real proxy behind it and nothing to install, but it puts an internet-facing entry point in front of otherwise private infrastructure, and adds API-key issuance and rotation, CORS configuration for local origins, shared state between developers, and a hosted component to operate. It also has no route back to a back end running on the developer's laptop, and answers nothing for step 1: the host shell still has to come from somewhere.
3. **Lightweight SDK.** A published package the remote app team installs, which mints the JWT locally and forwards requests to their own back end. Two shapes of this were discussed:
   - **3a. Library / dev-server rewrite.** The package hooks into the remote app's rsbuild dev server proxy: it intercepts requests, strips the `/api/{app}` prefix, and injects the JWT. No separate process to run, but the rewrite logic has to live in the remote app's dev server config, so it couples into that repository's build setup.
   - **3b. Standalone local proxy.** The developer installs the package and starts a proxy server with one command in the CLI. It mints the JWT and forwards to their local back end. The remote app's code and build config stay untouched, at the cost of an extra process the developer has to run.

   Both share the same limitation: they stand in for the proxy rather than running it, so what a developer tests is not the translation their requests will really go through, and either way every remote app repository takes on a dependency it must keep tracking as the host and proxy change.

## Decision

**Adopt the container image (option 1).** This was the group's preference by vote as the starting option.

Running locally sidesteps the private-link restriction outright rather than working around it with a public entry point, and keeping all traffic on localhost avoids CORS and API-key management entirely. Bundling the host shell and the proxy into a single artifact is the only one of the three that reproduces the whole chain, from the shell that mounts the remote through the real proxy that performs the JWT exchange and on to the developer's own back end, and it is what makes the one-line start and the default mock user achievable. It also keeps the remote app repositories clean: their only obligation is to expose a remote entry and a back end on localhost for the container to point at.

The hosted proxy and both SDK variants remain viable alternatives if the container approach proves too heavy in practice.

## Consequences

Positive:

- Remote app developers can exercise the full flow locally: their MFE mounted in the real host shell, requests going through the real proxy as a default user, and their own back end receiving them with a JWT, without credentials or network access to private infrastructure.
- Remote app repositories take on no new build-time dependency on the host.

Negative / follow-ups:

- Image size needs attention, since a large pull is a real ergonomic cost, and bundling the front end, back end, database, and Valkey into one artifact makes it a live concern rather than a theoretical one.
- We need an update and versioning mechanism so local images do not drift from the deployed host and proxy.
- Long-term maintenance of the bundled host and proxy falls to us; it is a new artifact to keep current, and it is consumed by teams outside this repository, so breaking it breaks them.
- We need to settle and document the two environment variables a developer sets to point the container at their local remote entry and their local back end, since that contract is the whole interface remote app teams see.
- If maintenance or image size becomes the dominant pain, revisit the hosted proxy or either SDK variant.
