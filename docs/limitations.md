# Restrictions and limitations

[Español](limitations.es.md) · [README](../README.md)

## Script Applications

- **The main HTML file must be a complete HTML document** (`<!DOCTYPE html><html><head>…</head><body>…</body></html>`).
  DXClient keeps the `<link>` and `<script>` elements of the `<head>` (rewriting their URLs to the files it
  stored in WCM) and the content of the `<body>`. With a bare fragment the outer element and the `<link>`
  elements are lost. Builds of Vite, webpack or HCL's `create-dx-script-app` already produce complete documents.
- **Scripts**: both inline scripts and files referenced from the `<head>` run. Files in the `<head>` run
  before the markup of the body exists: use `defer`, or wait for your root element.
- **Several instances on one page** share the page: select your root element by class or use DX's namespace
  token in IDs.
- **Each file is limited to 4.5 MB**: WCM stores each file in one element of at most 5,000,000 bytes, and a
  bigger file makes DX reject the push and leave an incomplete item. The operator checks the sizes before
  touching DX. Split the bundle (code splitting) or serve large files from the DAM.
- **Global CSS restyles the whole portal**: scope every rule to your application.
- **The WCM name is the key** (`spec.wcm.name`, default: the name of the resource): renaming it creates a
  second application; the old one stays in DX.
- **A restored application has a new content ID** (after being deleted in DX). Its pages are rebuilt
  automatically, with default viewer settings.
- **Edits in DX are overwritten** with `driftPolicy: Restore` (the default): the operator compares the last
  modification date of the content. Use `Report` where authors work on the application in DX.
- **Push or syndication, per environment**: if an environment receives the application by syndication, a push
  there creates a second, different item. Use `mode: Verify` in that environment.
- **WCM projects** (`wcm.projectContext`): the project must exist; publishing it is up to you.
- **Mutable tags** (`:latest`, `:main`): the operator resolves the digest on every pass, so a new image under
  the same tag is pushed at the next resync. Pin a digest (`repo@sha256:...`) for immutable deployments.
- **Substitutions** only apply to text files with the extensions listed in [how it works](how-it-works.md), and
  their values end up in files every visitor can download.
- **ConfigMap sources** are limited to 1 MiB in total, and keys cannot contain `/` (`assets__app.js` →
  `assets/app.js`).

## Pages

- **The user of the connection must be a DX administrator** (XMLAccess).
- **Changing the list of applications recreates the page** (XMLAccess cannot replace the components of a page
  in place): settings made on the viewers and changes to the layout are lost. Titles, access, theme, parent and
  position are updated in place.
- **Anonymous pages**: `access.anonymous: true` opens the page, but the Script Applications are WCM content and
  need anonymous users to have the User role on the **Script Application Library** (set it once in DX, *Web
  Content Libraries* → library → access). The operator cannot set WCM item permissions (DX offers no API for it).
- **Deleted pages are only marked** by DX and removed later by its cleanup task: until then, DX still lists
  references to them.
- One layout: the applications are stacked in one column (the `1Column` layout template by default).
- The URL in the status is only computed for pages under `ibm.portal.Home`.

## DAM

- Every file of the source becomes an asset named after its path (`img/logo.png` → `img-logo.png`).
- A file replaced in the DAM by someone (a new version) is not detected: only deleted assets and collections are.
- Collection access: the operator grants; it does not revoke permissions given in DX.
- Deleting a collection (policy Delete) ignores media references: pages and applications using its assets show
  broken links.

## Themes

- **WebDAV themes only** (the theme files in `dav:fs-type1/themes/`). Themes packaged as a WAR/EAR are not
  supported.
- **The registration is modelled on a theme of the same DX** (the base, default Portal 8.5: skins and settings), so the
  base must exist in that DX. With `base.copyFiles: true` (the default) its files are copied too, and the theme does not
  follow later changes of the base: force a reconciliation (or change the resource) to copy it again. A theme of its
  own (`base.copyFiles: false`) needs a source with every file of the theme: see the
  [EUROPESIP sample theme](https://github.com/europesip/hcl-dx-theme-sample).
- **The folder and the unique name are the keys**: renaming them creates a second theme; the old one stays in DX
  (unless the policy is Delete).
- **Applying a change uploads the whole folder** (about 430 files for Portal 8.5, a minute or two).
- **Edits made in DX to the files of the theme are not detected**; only a deleted registration or folder is.
- **Pages** pick the theme with `spec.theme` (its unique name); for the layouts of the new theme, point
  `layoutTemplate` at `dav:fs-type1/themes/<folder>/layout-templates/...`.

## DXDatabase

- Engines: **PostgreSQL, Oracle and SQL Server** (tested with PostgreSQL 17 / CloudNativePG, Oracle Database 23ai Free
  and SQL Server 2022). **DB2 is not supported yet**: its Go driver needs the native IBM client library (cgo), which
  the static image of the operator does not include.
- An existing server and an admin Secret readable by the operator (it may be in another namespace with
  `adminSecretNamespace`). The operator generates the password of the owner; if your DBA must keep the credentials,
  skip the DXDatabase and give the backend the Secret they provide (for example an ExternalSecret from your vault).
- Names: PostgreSQL lower-case identifiers; Oracle a lower-case schema name of up to 30 characters (the user is
  created in upper case); SQL Server letters, digits and `_`.
- The operator does not migrate schemas: that belongs to the backend (Flyway, Liquibase, its own start-up).

## Operator

- **Not an HCL product** and not supported by HCL. DXClient is used unmodified, as HCL distributes it.
- One active replica; DXClient runs are serialised, so many pushes queue up.
- Tested with HCL DX Compose CF239 and DXClient `v95_CF239_20261103-1404`. Use the DXClient of your DX level by
  building the image with `--build-arg DXCLIENT_IMAGE=hclcr.io/dx-public/dxclient:<tag>`.
- **DX with OpenID Connect (Keycloak, Entra ID...)**: supported from 0.1.2 (`dx.oidcCompatibility=false` in the chart turns it off, 0.1.3). With the OIDC client of Liberty the SSO
  cookie gets a generated name (`WAS_n...` instead of `LtpaToken2`) and the DX REST login answers without a cookie, so the
  operator falls back to the portal form login. The OIDC `authFilter` of DX must leave out `/wps/config` (XMLAccess,
  documented by HCL), `j_security_check` and requests with `Authorization: Basic` (DXClient).
- API `v1alpha1`: fields may still change before `v1`.
