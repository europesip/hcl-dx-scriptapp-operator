# Release history

**English** · [Español](CHANGELOG.es.md)

To go back to an earlier release: `helm upgrade dxop oci://ghcr.io/europesip/charts/hcl-dx-scriptapp-operator --version <version> --reuse-values`
(the CRDs are compatible across 0.1.x).

## 0.1.3 · 2026-11-08 · A switch for the OpenID Connect compatibility

- New chart value **`dx.oidcCompatibility`** (default `true`). `false` turns off the form login fallback of 0.1.2 and
  restores the behaviour of 0.1.1: only the REST login of DX. It sets the environment variable `DX_FORM_LOGIN`
  (`auto` / `false`) of the operator.

## 0.1.2 · 2026-11-08 · DX with OpenID Connect (Keycloak, Entra ID...)

- **What changed:** when the DX REST login (`/dx/api/core/v1/auth/login`) answers without a session cookie, the operator
  signs in with the portal login form (`<context root>/j_security_check`, without following the redirect) and uses that
  session for the Ring API and DAM calls.
- **Why:** when DX signs users in through OpenID Connect (the `openidConnectClient-1.0` feature of Liberty), Liberty names
  the SSO cookie `WAS_n<hash>` instead of `LtpaToken2`, and this cannot be configured (Open Liberty issue 18803). The DX
  REST login then answers 200 without a cookie, and every REST call of the operator got 401: the resources stayed in
  `DXError` and the DXConnection was never Ready. The Ring API and the DAM API accept the new cookie. Tested with DX
  Compose CF239 and Keycloak 26.8.
- **DX side:** the OIDC `authFilter` must leave out `/wps/config` (XMLAccess, as HCL documents), `j_security_check` and
  requests with `Authorization: Basic` (DXClient). See [docs/limitations.md](docs/limitations.md).
- **Without OIDC nothing changes:** the REST login returns `LtpaToken2` and no form login happens.
- **Going back:** chart 0.1.3 with `dx.oidcCompatibility=false`, or `--version 0.1.1`.

## 0.1.1 · 2026-11-08 · Adopted pages

- A DXPage that was already in DX exactly as declared (adopted) now records its applied state. Before, an adopted page
  ignored `deletionPolicy: Delete` (it stayed in DX when the resource was deleted) and later layout changes.

## 0.1.0 · 2026-11-08 · First release

- ScriptApplication, DXPage, DAMCollection, PostgresDatabase and DXConnection: Script Applications pushed with HCL
  DXClient (inside the operator image), pages with XMLAccess, DAM files, backend databases, drift detection and restore,
  Argo CD health checks.
