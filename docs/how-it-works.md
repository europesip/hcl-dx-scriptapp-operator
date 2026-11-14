# How the operator works

[Español](how-it-works.es.md) · [README](https://github.com/europesip/hcl-dx-scriptapp-operator#readme)

## The pieces

```
                          Kubernetes                                         HCL DX
 ┌───────────────────────────────────────────────────────┐        ┌──────────────────────────────┐
 │ DXConnection ──┐                                      │        │                              │
 │ DXScriptApplication ─┐  ┌──────────────────────────┐   │ push   │ WCM: Script Application      │
 │ DXPage ────────────┼─►│ operator (one pod)       │───┼───────►│      Library (DXClient)      │
 │ DXDamCollection ─────┤  │  · controllers           │   │ XML    │ Pages (XMLAccess /wps/config)│
 │ DXDatabase ──┘  │  · DXClient (/dxclient)  │───┼───────►│ DAM (REST /dx/api/dam/v1)    │
 │                       │  · source cache          │   │ check  │ Ring API (/dx/api/core/v1)   │
 │ Deployments, Jobs ───►│  (dependsOn)             │───┼───────►│                              │
 │                       └──────────┬───────────────┘   │        └──────────────────────────────┘
 │                                  │ SQL                │
 │                                  ▼                    │
 │                         PostgreSQL (existing)         │
 └───────────────────────────────────────────────────────┘
```

- **One image**: HCL's public DXClient image plus the operator binary. DXClient runs inside the operator for
  pushes; no Job or extra image is created.
- **Sources** are read by the operator itself: OCI images straight from the registry (their layers, with
  `go-containerregistry`; nothing runs), ConfigMaps, or archives over HTTP(S). They are cached by revision
  (image digest, ConfigMap hash, archive SHA256).
- **DX APIs**: Ring API (sign-in, WCM content by ID and by name), WCM REST v2 (delete), XMLAccess endpoint
  `/wps/config` (pages), DAM REST API (collections, assets, renditions), Ring access API (DAM permissions).

## The reconciliation loop

Every resource is reconciled when it changes, when an object it refers to changes (its ConfigMap, the DAM
collection of a substitution, the Deployment it depends on, its connection...), and every `resyncInterval` of
its DXConnection (default 10 minutes). Each pass:

1. **Deletion** (if the resource is being deleted): with `deletionPolicy: Delete`, remove the object from DX;
   then release the finalizer. `Retain` (the default) leaves DX as it is.
2. **Suspend**: `spec.suspend: true` stops here.
3. **Dependencies**: every `dependsOn`, plus the implicit ones (the DAM collections of substitutions, the
   Script Applications of a page). If one is not ready: `Ready=False, reason Waiting`, retry in 20 seconds.
4. **Connection**: the DXConnection must be Ready.
5. **Desired state**: what the resource declares, as a hash.
6. **Actual state** in DX, and the decision: create, update, restore, report, or nothing.
7. **Status and events**: conditions `Ready`, `Drifted` and `DependenciesReady`, plus Kubernetes events.

Writes to the status are merge patches, so concurrent passes do not lose what was done in DX.

### DXScriptApplication

- **Prepare**: get the source; copy it to a working folder; replace the `${NAME}` placeholders of
  `substitutions` in the text files (`.html .htm .js .mjs .css .json .txt .svg .xml`; other `${...}` are left
  alone); check that the main HTML file exists and that no file is over 4.5 MB.
- **Hash** of every prepared file plus the WCM settings (name, title, site area, main file, project) and the
  force annotation.
- **Decide**:
  - no content ID yet → push (`Created`);
  - hash changed → push (`Updated`);
  - otherwise read the content in DX by ID (Ring API): 404 → it was **deleted** in DX; a different `updated`
    date than the one recorded after the last push → it was **modified** in DX. With `driftPolicy: Restore` push
    again (`Restored`; after a deletion WCM creates a new item, with a new content ID); with `Report` mark
    `Drifted` and do nothing.
- **Push**: `dxclient deploy-scriptapplication push`, with the connection in DXClient's configuration file
  (removed afterwards). The push is accepted only if DXClient exits 0 **and** DX's answer has
  `"status":"success"` (DXClient prints "successful" even when DX rejects the content). Temporary errors of a DX
  that is starting (Service Unavailable, Bad Gateway, connection reset...) are retried 5 times, 20 s apart.
- **Record** the content ID, the hash, the source revision, the `updated` date, the size and a history of the
  last 10 pushes.
- **Mode Verify** never pushes: it finds the content by its WCM name and reports Ready or `NotFound`.
- **Delete** (policy Delete): WCM REST v2 `DELETE /wcmrest-v2/contents/{id}` (DXClient's
  `undeploy-scriptapplication` refuses while a deleted page still references the application: DX removes
  deleted pages later, with its cleanup task).

### DXPage

- **Desired**: the content IDs of its Script Applications (in order), titles, parent, friendly name, layout
  template, skin, theme, access and position, as a hash. A second hash covers only the list of applications
  (the layout).
- **Actual**: an XMLAccess export by unique name. `EJPXA0240E` means the page does not exist; any other error
  is an error (a temporary failure must not be taken for a deleted page, or the page would be created twice).
  The hash applied by the operator is stored in the page itself, as the parameter `dxop.hash`.
- **Decide**: missing and never created → create with its layout; missing after being created → deleted in DX
  (restore or report); list of applications changed → delete and create again (XMLAccess cannot replace the
  components of a page in place); other settings changed → update titles, parameters and access without
  touching the layout (so the settings people make on the page survive); `dxop.hash` in DX different from the
  one applied → someone changed the settings in DX (restore or report).
- A Script Application that gets a new content ID (restored after a deletion) triggers its pages: the layout
  hash changes and the page is rebuilt.

### DXTheme

- **Desired**: the spec (folder, unique name, titles, base, profile, layout, head HTML) and the revision of its
  source, as a hash.
- **Actual**: an XMLAccess export of the theme by unique name (the applied hash is kept in the theme as the parameter
  `dxop.hash`) and a WebDAV listing of its folder (`dav:fs-type1/themes/<folder>/`).
- **Apply**: with `base.copyFiles: false` (a theme of its own) only the files of the source; otherwise download every
  file of the base theme over WebDAV first and put the files of the source on top (same paths), then
  insert the head HTML before `</head>` in `theme.html`, `theme_sidenav.html` and their language versions, upload
  the folder (MKCOL + PUT), and register the theme with a copy of the registration of the base theme (new unique
  name, templates pointing at the new folder, `default="false"`, only the declared titles, and `profile` / `layout`
  when they are set).
- **Drift**: registration or folder missing in DX → upload and register again (`Restore`) or report.
- **Delete** (policy Delete): delete the registration (XMLAccess) and the folder (WebDAV). Pages that use it fall
  back to the default theme.
- WebDAV and XMLAccess use basic auth, so the theme works the same with DX behind OpenID Connect.
- DXClient `deploy-theme` is not used: its registration step fails on DX Compose CF239.

### DXDamCollection

- Find the collection by the ID in the status, or by name (so an existing one is adopted), or create it; give
  the User role to anonymous and/or all authenticated users.
- For each file of the source (assets named after their path, `/` → `-`): unchanged (same SHA256 as recorded) →
  nothing; changed → `PUT` a new version of the same asset (same ID and URL); missing in DX → upload again
  (restore or report); an asset with that name already there → adopt it. Then wait for its original rendition
  and record its URL.
- `prune: true` deletes the assets that are no longer in the source.
- Delete (policy Delete): the collection, ignoring media references.

### DXDatabase

- A Secret `<name>-db` (or `secretName`) with the connection details; the password is generated once and kept,
  and the Secret is owned by the resource.
- With the admin Secret, by engine (checked every 30 minutes):
  - **PostgreSQL**: create or alter the role (login, password), create the database owned by it if missing, create
    the extensions.
  - **Oracle** (`oracle.service`, the PDB): the database is a schema: create the user (default tablespace and quota)
    with the privileges to create tables, views, sequences, procedures, triggers, types and synonyms; an existing user
    gets the password again.
  - **SQL Server**: create or alter the login, create the database if missing, its user for the login and the
    `db_owner` membership.
- Delete (policy Delete): PostgreSQL `DROP DATABASE ... WITH (FORCE)` and `DROP ROLE`; Oracle `DROP USER ... CASCADE`;
  SQL Server `DROP DATABASE` (single user first) and `DROP LOGIN`.
- The Secret has the same keys for every engine: `engine`, `username`, `password`, `host`, `port`, `database`,
  `uri` and `jdbcUrl` (plus `service` with Oracle).

### DXConnection

Signs in to DX with a fresh session (`/dx/api/core/v1/auth/login`) and checks that DXClient can run (licence
accepted, files present). Changes in its Secrets trigger a new check.

## Concurrency and performance

- One active replica (leader election). DXClient runs are **serialised** (one at a time): DXClient keeps its
  settings in its own folder, and DX does not like parallel pushes either. REST calls run in parallel.
- A push takes a few seconds; a page about one second; a check (resync) is one or two REST calls per resource.
- REST sessions are reused per connection while its Secrets do not change.

## Forcing and pausing

- `metadata.annotations["dx.europesip.com/reconcile"]`: change its value to reconcile now; for a Script
  Application it also forces a push (the value is part of the hash).
- `spec.suspend: true`: nothing is done until it is false again.

## Status and events

```bash
kubectl get dxscriptapplication time-off -o yaml        # status.contentId, history, conditions
kubectl get events --field-selector involvedObject.name=time-off
```

Events: `Pushed` (Created / Updated / Restored), `Drifted`, `PushFailed`, `Created`, `Rebuilt`, `Synced`,
`Deleted`, `Undeployed`, `Dropped`, `Connected`, `LoginFailed`...
