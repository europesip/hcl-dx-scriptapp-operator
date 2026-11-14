# Release history

**English** · [Español](CHANGELOG.es.md)

To go back to an earlier release: `helm upgrade dxop oci://ghcr.io/europesip/charts/hcl-dx-scriptapp-operator --version <version> --reuse-values`
(0.3.0 renamed the resources: see its notes).

## Unreleased

- **TechDocs for Backstage** (#6): `mkdocs.yml` and a docs home page (`docs/index.md`, `docs/index.es.md`), so the
  EUROPESIP Internal Developer Portal shows the operator documentation. Links from `docs/` to files outside it (and to
  `docs/specs/`) are absolute GitHub links, also in `hack/gen-reference.py`. No change to the operator, the CRDs or
  the chart.

## 0.3.2 · 2026-11-12 · OIDC session cookie WAS_p...

- **Fix:** with OpenID Connect, Liberty names the DX SSO cookie `WAS_<letter><hash>`, and the letter depends on the DX
  (`WAS_n...` on the lab DX, `WAS_p...` on another DX Compose CF239). The operator only accepted `WAS_n...`: the
  `DXConnection` stayed `LoginFailed` ("DX did not open a session ... form login HTTP 302"). Any `WAS_` cookie is the
  session now (`WASReqURL...` and `WASOidc...` are not). Tested with DX Compose CF239 + Keycloak 26.3: `DXConnection`
  Ready, sample 01 pushed and its page created.

## 0.3.1 · 2026-11-08 · XMLAccess, one call at a time

- **Fix:** DX answers concurrent XMLAccess requests with an empty HTTP 200 now and then; the resource went to `DXError`
  (and ArgoCD showed it Degraded) until the next check. The operator now sends one XMLAccess request at a time to each
  DX, and retries an empty answer twice (after 2 and 4 seconds). Seen on DX Compose CF239 when a page and a theme were
  checked at the same moment.

## 0.3.0 · 2026-11-08 · DX prefix, DXDatabase for every engine, themes of their own

**Breaking: the resources were renamed** so that every one has the DX prefix:

| Before | Now | Short name |
|---|---|---|
| `ScriptApplication` | `DXScriptApplication` | `dxapp`, `dxsa` |
| `DAMCollection` | `DXDamCollection` | `dxdam` |
| `PostgresDatabase` | `DXDatabase` (`superuserSecret` → `adminSecret`) | `dxdb` |

`DXConnection`, `DXPage` and `DXTheme` keep their names. **Migrating from 0.2:** apply the CRDs of 0.3.0, change the
`kind` of your resources (and `superuserSecret` / `superuserSecretNamespace` → `adminSecret` / `adminSecretNamespace`)
and apply them again: the operator finds the existing content in DX by its name (WCM name, collection name, database)
and adopts it. Then delete the old resources (their finalizers are only removed by an operator that knows them: with
0.3.0 running, patch them out with `kubectl patch ... --type merge -p '{"metadata":{"finalizers":null}}'`; nothing is
removed from DX) and the old CRDs.

- **DXDatabase**: PostgreSQL, **Oracle** (`oracle.service`; the database is a schema) and **SQL Server**
  (`sqlServer.encrypt`, `trustServerCertificate`), with `engine`. Same Secret for every engine (`engine`, `username`,
  `password`, `host`, `port`, `database`, `uri`, `jdbcUrl`). Tested with PostgreSQL 17, Oracle Database 23ai Free and SQL
  Server 2022: create, sign in as the owner, reconcile again, delete. DB2 is not supported yet (its Go driver needs cgo).
- **DXTheme**: `base.copyFiles: false` uploads only the files of the source (a theme of its own, nothing copied from the
  themes of HCL); `profile` and `layout` set the resource aggregation profile and the default layout. Sample 09 now
  installs the [EUROPESIP sample theme](https://github.com/europesip/hcl-dx-theme-sample) from its release.

 2026-11-08 · Themes

(0.2.0 was tagged but never published: its CI failed on a generated file header.)

- New resource **DXTheme** (short name `dxtheme`): a WebDAV theme of its own. The operator copies a base theme of the
  same DX (default: Portal 8.5) over WebDAV, adds the files of its `source` (ConfigMap, image or URL, as the Script
  Applications) and the HTML of `headHTML` in the `<head>` of its templates, uploads the folder and registers the
  theme with XMLAccess (copy of the registration of the base). Drift (registration or folder deleted in DX) is
  restored or reported; `deletionPolicy: Delete` removes the registration and the folder.
- Pages use it with `spec.theme` (its unique name, default `dxop.<namespace>.<name>`), and can wait for it with
  `dependsOn: [{kind: DXTheme, name: ...}]` (new dependency kind).
- New sample [09-theme](samples/09-theme/theme.yaml) and ArgoCD health check for DXTheme.
- Works with and without OpenID Connect (basic auth for WebDAV and XMLAccess). Tested with DX Compose CF239, with
  and without Keycloak.
- **Fix:** after a restart of DX (for example a `helm upgrade` of DX) the old SSO cookie stayed in the operator, the new
  login skipped the form login and every call answered 401 until the operator was restarted (resources in `DXError`).
  Every login now starts with an empty cookie jar.
- New CRD: Helm only installs CRDs on the first install, so with an existing release apply
  `charts/hcl-dx-scriptapp-operator/crds/` before `helm upgrade`. The other CRDs only gain the `DXTheme` dependency kind.

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
