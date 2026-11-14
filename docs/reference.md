# API reference (dx.europesip.com/v1alpha1)

[README](https://github.com/europesip/hcl-dx-scriptapp-operator#readme) · generated from the CRDs by `hack/gen-reference.py`

## DXConnection

DXConnection is a connection to an HCL DX server, shared by the resources of a namespace.

Short names: `dxconn`

### spec

| Field | Type | Default | Description |
|---|---|---|---|
| `caSecret` | object |  | CASecret: Secret with the key "ca.crt", for a self-signed certificate or a private CA. |
| `caSecret.name` | string (required) |  | Name of the object. |
| `contentHandlerPath` | string | `/wps/mycontenthandler` | ContentHandlerPath of DX (default /wps/mycontenthandler). |
| `credentialsSecret` | object (required) |  | CredentialsSecret: Secret with the keys "username" and "password" of a DX user. Script Applications need Editor access to the Script Application Library; pages (XMLAccess) and DAM need a DX administrator. |
| `credentialsSecret.name` | string (required) |  | Name of the object. |
| `insecureSkipTLSVerify` | boolean |  | InsecureSkipTLSVerify: do not verify the certificate at all (labs only). |
| `resyncInterval` | string | `10m` | ResyncInterval: how often every resource that uses this connection is checked against DX, to detect and correct drift (default 10m, minimum 1m). |
| `url` | string (required) |  | URL of DX as the operator reaches it, for example https://dx.example.com (normally the public host). |
| `virtualPortalContext` | string |  | VirtualPortalContext: push Script Applications into this virtual portal. |

### status

| Field | Type | Default | Description |
|---|---|---|---|
| `conditions` | []object |  |  |
| `conditions[].lastTransitionTime` | string (required) |  | lastTransitionTime is the last time the condition transitioned from one status to another. This should be when the underlying condition changed. If that is not known, then using the time when the API field changed is acceptable. |
| `conditions[].message` | string (required) |  | message is a human readable message indicating details about the transition. This may be an empty string. |
| `conditions[].observedGeneration` | integer |  | observedGeneration represents the .metadata.generation that the condition was set based upon. For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date with respect to the current state of the instance. |
| `conditions[].reason` | string (required) |  | reason contains a programmatic identifier indicating the reason for the condition's last transition. Producers of specific condition types may define expected values and meanings for this field, and whether the values are considered a guaranteed API. The value should be a CamelCase string. This field may not be empty. |
| `conditions[].status` | True \| False \| Unknown (required) |  | status of the condition, one of True, False, Unknown. |
| `conditions[].type` | string (required) |  | type of condition in CamelCase or in foo.example.com/CamelCase. |
| `lastCheckTime` | string |  | LastCheckTime of the connection. |
| `observedGeneration` | integer |  |  |

## DXScriptApplication

DXScriptApplication is an HCL DX Script Application, kept in DX as the resource declares.

Short names: `dxapp`, `dxsa`

### spec

| Field | Type | Default | Description |
|---|---|---|---|
| `connectionRef` | object (required) |  | ConnectionRef: the DXConnection to use. |
| `connectionRef.name` | string (required) |  | Name of the object. |
| `deletionPolicy` | Retain \| Delete | `Retain` | DeletionPolicy: Retain (default) leaves the application in DX when the resource is deleted; Delete removes it (pages that use it then show an error). |
| `dependsOn` | []object |  | DependsOn: objects that must be ready first (for example the Deployment of the backend). |
| `dependsOn[].apiVersion` | string |  | APIVersion of the object, for example apps/v1 (default) or dx.europesip.com/v1alpha1. |
| `dependsOn[].kind` | Deployment \| StatefulSet \| Job \| DXScriptApplication \| DXPage \| DXDamCollection \| DXDatabase \| DXTheme (required) |  | Kind: Deployment, StatefulSet, Job, DXScriptApplication, DXPage, DXDamCollection, DXDatabase or DXTheme. |
| `dependsOn[].name` | string (required) |  | Name of the object, in the same namespace. |
| `driftPolicy` | Restore \| Report | `Restore` | DriftPolicy: Restore (default) pushes again if the application was deleted or edited in DX; Report only reports it. |
| `mode` | Push \| Verify | `Push` | Mode: Push (default) or Verify. |
| `source` | object (required) |  | Source of the built application (the content of dist/). |
| `source.configMap` | object |  | ConfigMap: every key of the ConfigMap is a file (data and binaryData). Handy for small applications: a ConfigMap and its Secret are limited to 1 MiB. Keys cannot contain "/", so a key "assets__app.js" becomes the file "assets/app.js". |
| `source.configMap.name` | string (required) |  | Name of the ConfigMap. |
| `source.image` | object |  | Image: an OCI image (a "bundle image") that carries the files. The operator reads its layers directly from the registry: nothing runs in the cluster. Use a digest (repo@sha256:...) for immutable deployments. |
| `source.image.path` | string | `/app` | Path inside the image where the files are (default /app). |
| `source.image.pullSecrets` | []object |  | PullSecrets: Secrets of type kubernetes.io/dockerconfigjson for a private registry. |
| `source.image.pullSecrets[].name` | string (required) |  | Name of the object. |
| `source.image.ref` | string (required) |  | Reference of the image, for example ghcr.io/acme/my-app:1.2.0 or ghcr.io/acme/my-app@sha256:... |
| `source.url` | object |  | URL: a .tar.gz, .tgz or .zip archive downloaded over HTTP(S), for example a CI artifact or a GitHub release asset. |
| `source.url.path` | string |  | Path inside the archive where the files are (default: the root). |
| `source.url.sha256` | string |  | SHA256 of the archive (hex). Recommended: the download is rejected if it does not match, and the operator only downloads it again when this value changes. |
| `source.url.url` | string (required) |  | URL of a .tar.gz, .tgz or .zip archive. |
| `substitutions` | []object |  | Substitutions of ${NAME} placeholders in the text files. |
| `substitutions[].name` | string (required) |  | Name of the placeholder: ${NAME} in the files. |
| `substitutions[].value` | string |  | Value, as is. |
| `substitutions[].valueFrom` | object |  | ValueFrom: a key of a ConfigMap or a Secret, or the URL of an asset of a DXDamCollection (which then becomes a dependency). |
| `substitutions[].valueFrom.configMapKeyRef` | object |  | ConfigMapKeySelector selects a key of a ConfigMap in the same namespace. |
| `substitutions[].valueFrom.configMapKeyRef.key` | string (required) |  |  |
| `substitutions[].valueFrom.configMapKeyRef.name` | string (required) |  |  |
| `substitutions[].valueFrom.damAsset` | object |  | DAMAsset: the URL of an asset of a DXDamCollection in the same namespace. |
| `substitutions[].valueFrom.damAsset.asset` | string (required) |  | Asset: file name of the asset in the collection. |
| `substitutions[].valueFrom.damAsset.collection` | string (required) |  | Collection: name of the DXDamCollection resource. |
| `substitutions[].valueFrom.secretKeyRef` | object |  | SecretKeyRef: remember that the value ends up in a file every visitor can download. |
| `substitutions[].valueFrom.secretKeyRef.key` | string (required) |  |  |
| `substitutions[].valueFrom.secretKeyRef.name` | string (required) |  |  |
| `suspend` | boolean |  | Suspend: stop reconciling (nothing is pushed or checked). |
| `wcm` | object |  | WCM settings. |
| `wcm.mainHtmlFile` | string | `index.html` | MainHTMLFile of the application (default index.html). |
| `wcm.name` | string |  | Name of the WCM content: the key that makes a push an update. Default: the name of the resource. Do not change it after the first push (a new name creates a second application). |
| `wcm.projectContext` | string |  | ProjectContext: push into this WCM project instead of publishing at once (the project must exist). |
| `wcm.siteArea` | string | `Script Application Library/Script Applications/` | SiteArea where it is created (default "Script Application Library/Script Applications/"). |
| `wcm.title` | string |  | Title shown in DX (default: the WCM name). |

### status

| Field | Type | Default | Description |
|---|---|---|---|
| `appliedHash` | string |  | AppliedHash: hash of everything pushed (files after substitutions and WCM settings). |
| `conditions` | []object |  |  |
| `conditions[].lastTransitionTime` | string (required) |  | lastTransitionTime is the last time the condition transitioned from one status to another. This should be when the underlying condition changed. If that is not known, then using the time when the API field changed is acceptable. |
| `conditions[].message` | string (required) |  | message is a human readable message indicating details about the transition. This may be an empty string. |
| `conditions[].observedGeneration` | integer |  | observedGeneration represents the .metadata.generation that the condition was set based upon. For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date with respect to the current state of the instance. |
| `conditions[].reason` | string (required) |  | reason contains a programmatic identifier indicating the reason for the condition's last transition. Producers of specific condition types may define expected values and meanings for this field, and whether the values are considered a guaranteed API. The value should be a CamelCase string. This field may not be empty. |
| `conditions[].status` | True \| False \| Unknown (required) |  | status of the condition, one of True, False, Unknown. |
| `conditions[].type` | string (required) |  | type of condition in CamelCase or in foo.example.com/CamelCase. |
| `contentId` | string |  | ContentID of the application in WCM. |
| `files` | integer |  | Files and Size of the last push. |
| `history` | []object |  | History of the last pushes (newest first, at most 10): to go back, set the source of one of them. |
| `history[].reason` | string (required) |  |  |
| `history[].sourceRevision` | string (required) |  |  |
| `history[].time` | string (required) |  |  |
| `lastCheckTime` | string |  |  |
| `lastPushTime` | string |  | LastPushTime and LastCheckTime. |
| `observedGeneration` | integer |  |  |
| `size` | string |  |  |
| `sourceRevision` | string |  | SourceRevision: digest of the source that was pushed (image digest, ConfigMap hash or archive SHA256). |
| `wcmLastModified` | string |  | WCMLastModified: last modification date of the content in WCM after the operator's push. A different date means somebody changed it in DX. |

## DXPage

DXPage is a page of HCL DX that shows Script Applications, kept in DX as the resource declares.

Short names: `dxp`

### spec

| Field | Type | Default | Description |
|---|---|---|---|
| `access` | object |  | Access: who can see the page (default: all authenticated users). |
| `access.anonymous` | boolean |  | Anonymous: visitors who are not signed in. |
| `access.authenticated` | boolean | `True` | Authenticated: all authenticated portal users (default true). |
| `applications` | []object (required) |  | Applications shown on the page, one under the other: names of DXScriptApplication resources in the same namespace (each one in a Web Content Viewer). They become dependencies of the page. |
| `applications[].name` | string (required) |  | Name of the object. |
| `connectionRef` | object (required) |  | ConnectionRef: the DXConnection to use (its user must be a DX administrator: pages use XMLAccess). |
| `connectionRef.name` | string (required) |  | Name of the object. |
| `deletionPolicy` | Retain \| Delete | `Retain` | DeletionPolicy: Retain (default) leaves the page in DX when the resource is deleted; Delete removes it. |
| `dependsOn` | []object |  | DependsOn: other objects that must be ready first. |
| `dependsOn[].apiVersion` | string |  | APIVersion of the object, for example apps/v1 (default) or dx.europesip.com/v1alpha1. |
| `dependsOn[].kind` | Deployment \| StatefulSet \| Job \| DXScriptApplication \| DXPage \| DXDamCollection \| DXDatabase \| DXTheme (required) |  | Kind: Deployment, StatefulSet, Job, DXScriptApplication, DXPage, DXDamCollection, DXDatabase or DXTheme. |
| `dependsOn[].name` | string (required) |  | Name of the object, in the same namespace. |
| `driftPolicy` | Restore \| Report | `Restore` | DriftPolicy: Restore (default) recreates the page if it was deleted in DX; Report only reports it. |
| `friendlyName` | string |  | FriendlyName of the page in its URL (default: the name of the resource). |
| `layoutTemplate` | string | `dav:fs-type1/themes/Portal8.5/layout-templates/1Column` | LayoutTemplate of the theme (default: the 1 column layout of the Portal 8.5 theme). |
| `ordinal` | integer |  | Ordinal of the page among its siblings (optional). |
| `parent` | string | `ibm.portal.Home` | Parent: unique name of the parent page (default ibm.portal.Home). |
| `skin` | string | `ibm.portal.85NoSkin` | Skin of the viewers (default ibm.portal.85NoSkin). |
| `suspend` | boolean |  | Suspend: stop reconciling. |
| `theme` | string |  | Theme: unique name of a theme for this page (default: inherited). |
| `titles` | object (required) |  | Titles by locale, for example {en: "Time off", es: "Vacaciones"}. |
| `uniqueName` | string |  | UniqueName of the page in DX: the key that makes an update find the same page. Default: dxop.<namespace>.<name>. Do not change it after the page is created. |

### status

| Field | Type | Default | Description |
|---|---|---|---|
| `appliedHash` | string |  | AppliedHash: hash of the settings applied with XMLAccess. |
| `conditions` | []object |  |  |
| `conditions[].lastTransitionTime` | string (required) |  | lastTransitionTime is the last time the condition transitioned from one status to another. This should be when the underlying condition changed. If that is not known, then using the time when the API field changed is acceptable. |
| `conditions[].message` | string (required) |  | message is a human readable message indicating details about the transition. This may be an empty string. |
| `conditions[].observedGeneration` | integer |  | observedGeneration represents the .metadata.generation that the condition was set based upon. For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date with respect to the current state of the instance. |
| `conditions[].reason` | string (required) |  | reason contains a programmatic identifier indicating the reason for the condition's last transition. Producers of specific condition types may define expected values and meanings for this field, and whether the values are considered a guaranteed API. The value should be a CamelCase string. This field may not be empty. |
| `conditions[].status` | True \| False \| Unknown (required) |  | status of the condition, one of True, False, Unknown. |
| `conditions[].type` | string (required) |  | type of condition in CamelCase or in foo.example.com/CamelCase. |
| `lastApplyTime` | string |  |  |
| `lastCheckTime` | string |  |  |
| `layoutHash` | string |  | LayoutHash: hash of the list of applications the layout was built for. The layout (the viewers) is only rebuilt when that list changes, so the settings people make on the page survive other updates. |
| `observedGeneration` | integer |  |  |
| `uniqueName` | string |  | UniqueName of the page in DX. |
| `url` | string |  | URL of the page (for authenticated users), when the parent is the Home page. |

## DXTheme

DXTheme is a theme of HCL DX Compose (WebDAV based), kept in DX as the resource declares.

Short names: `dxtheme`

### spec

| Field | Type | Default | Description |
|---|---|---|---|
| `base` | object |  | Base theme copied first (default: Portal 8.5). |
| `base.copyFiles` | boolean |  | CopyFiles: true (default) copies the files of the base theme first and puts the source on top; false uploads only the files of the source (a complete theme of its own), and the base is used only as the model of the registration. |
| `base.folder` | string | `Portal8.5` | Folder of the base theme in the WebDAV store (default: Portal8.5, the HCL DX Portal 8.5 theme). |
| `base.uniqueName` | string | `ibm.portal.85Theme` | UniqueName of the base theme registration (default: ibm.portal.85Theme). |
| `connectionRef` | object (required) |  | ConnectionRef: the DXConnection to use. |
| `connectionRef.name` | string (required) |  | Name of the object. |
| `deletionPolicy` | Retain \| Delete | `Retain` | DeletionPolicy: Retain (default) leaves the theme in DX when the resource is deleted; Delete removes its registration and its folder (pages that use it fall back to the default theme). |
| `dependsOn` | []object |  | DependsOn: other objects that must be ready first. |
| `dependsOn[].apiVersion` | string |  | APIVersion of the object, for example apps/v1 (default) or dx.europesip.com/v1alpha1. |
| `dependsOn[].kind` | Deployment \| StatefulSet \| Job \| DXScriptApplication \| DXPage \| DXDamCollection \| DXDatabase \| DXTheme (required) |  | Kind: Deployment, StatefulSet, Job, DXScriptApplication, DXPage, DXDamCollection, DXDatabase or DXTheme. |
| `dependsOn[].name` | string (required) |  | Name of the object, in the same namespace. |
| `description` | string |  | Description of the theme. |
| `driftPolicy` | Restore \| Report | `Restore` | DriftPolicy: Restore (default) uploads and registers it again if the theme or its folder was deleted in DX; Report only reports it. |
| `folder` | string |  | Folder of the theme in the WebDAV store (dav:fs-type1/themes/<folder>/). Default: the name of the resource. |
| `headHTML` | string |  | HeadHTML: HTML inserted before </head> in every theme template (theme.html, theme_sidenav.html and their language versions): a <style>, a <link> to a file of the source (/wps/mycontenthandler/dav/fs-type1/themes/<folder>/...), a <script>... |
| `layout` | string |  | Layout: default layout template of the theme, relative to its folder (for example layout-templates/1Column). Default: the one of the base theme, in the new folder. |
| `profile` | string |  | Profile: resource aggregation profile of the theme, relative to its folder (for example profiles/profile_europesip.json). Default: the one of the base theme. |
| `source` | object |  | Source: files added to (or replacing in) the copy of the base theme, at the same paths (for example css/my.css or images/logo.png). Optional. |
| `source.configMap` | object |  | ConfigMap: every key of the ConfigMap is a file (data and binaryData). Handy for small applications: a ConfigMap and its Secret are limited to 1 MiB. Keys cannot contain "/", so a key "assets__app.js" becomes the file "assets/app.js". |
| `source.configMap.name` | string (required) |  | Name of the ConfigMap. |
| `source.image` | object |  | Image: an OCI image (a "bundle image") that carries the files. The operator reads its layers directly from the registry: nothing runs in the cluster. Use a digest (repo@sha256:...) for immutable deployments. |
| `source.image.path` | string | `/app` | Path inside the image where the files are (default /app). |
| `source.image.pullSecrets` | []object |  | PullSecrets: Secrets of type kubernetes.io/dockerconfigjson for a private registry. |
| `source.image.pullSecrets[].name` | string (required) |  | Name of the object. |
| `source.image.ref` | string (required) |  | Reference of the image, for example ghcr.io/acme/my-app:1.2.0 or ghcr.io/acme/my-app@sha256:... |
| `source.url` | object |  | URL: a .tar.gz, .tgz or .zip archive downloaded over HTTP(S), for example a CI artifact or a GitHub release asset. |
| `source.url.path` | string |  | Path inside the archive where the files are (default: the root). |
| `source.url.sha256` | string |  | SHA256 of the archive (hex). Recommended: the download is rejected if it does not match, and the operator only downloads it again when this value changes. |
| `source.url.url` | string (required) |  | URL of a .tar.gz, .tgz or .zip archive. |
| `suspend` | boolean |  | Suspend: stop reconciling. |
| `titles` | object (required) |  | Titles by locale (for example {en: "My theme", es: "Mi tema"}). At least one. |
| `uniqueName` | string |  | UniqueName of the theme registration (default dxop.<namespace>.<name>). Pages refer to it (DXPage spec.theme). |

### status

| Field | Type | Default | Description |
|---|---|---|---|
| `appliedHash` | string |  | AppliedHash: hash of what was uploaded and registered (also kept as the theme parameter dxop.hash). |
| `conditions` | []object |  |  |
| `conditions[].lastTransitionTime` | string (required) |  | lastTransitionTime is the last time the condition transitioned from one status to another. This should be when the underlying condition changed. If that is not known, then using the time when the API field changed is acceptable. |
| `conditions[].message` | string (required) |  | message is a human readable message indicating details about the transition. This may be an empty string. |
| `conditions[].observedGeneration` | integer |  | observedGeneration represents the .metadata.generation that the condition was set based upon. For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date with respect to the current state of the instance. |
| `conditions[].reason` | string (required) |  | reason contains a programmatic identifier indicating the reason for the condition's last transition. Producers of specific condition types may define expected values and meanings for this field, and whether the values are considered a guaranteed API. The value should be a CamelCase string. This field may not be empty. |
| `conditions[].status` | True \| False \| Unknown (required) |  | status of the condition, one of True, False, Unknown. |
| `conditions[].type` | string (required) |  | type of condition in CamelCase or in foo.example.com/CamelCase. |
| `files` | integer |  | Files uploaded to the WebDAV folder. |
| `lastApplyTime` | string |  |  |
| `observedGeneration` | integer |  |  |
| `sourceRevision` | string |  | SourceRevision of the files that were added. |
| `uniqueName` | string |  | UniqueName of the registered theme. |

## DXDamCollection

DXDamCollection is a collection of the HCL DX Digital Asset Management, kept as the resource declares.

Short names: `dxdam`

### spec

| Field | Type | Default | Description |
|---|---|---|---|
| `access` | object |  | Access: who can read the assets (default: all authenticated users; anonymous for public images). |
| `access.anonymous` | boolean |  | Anonymous: visitors who are not signed in. |
| `access.authenticated` | boolean | `True` | Authenticated: all authenticated portal users (default true). |
| `collectionName` | string |  | CollectionName in the DAM (default: the name of the resource). The key that finds the same collection. |
| `connectionRef` | object (required) |  | ConnectionRef: the DXConnection to use. |
| `connectionRef.name` | string (required) |  | Name of the object. |
| `deletionPolicy` | Retain \| Delete | `Retain` | DeletionPolicy: Retain (default) leaves the collection in the DAM when the resource is deleted; Delete removes it with its assets. |
| `dependsOn` | []object |  | DependsOn: other objects that must be ready first. |
| `dependsOn[].apiVersion` | string |  | APIVersion of the object, for example apps/v1 (default) or dx.europesip.com/v1alpha1. |
| `dependsOn[].kind` | Deployment \| StatefulSet \| Job \| DXScriptApplication \| DXPage \| DXDamCollection \| DXDatabase \| DXTheme (required) |  | Kind: Deployment, StatefulSet, Job, DXScriptApplication, DXPage, DXDamCollection, DXDatabase or DXTheme. |
| `dependsOn[].name` | string (required) |  | Name of the object, in the same namespace. |
| `description` | string |  | Description of the collection. |
| `driftPolicy` | Restore \| Report | `Restore` | DriftPolicy: Restore (default) uploads again what was deleted or replaced in DX; Report only reports it. |
| `prune` | boolean |  | Prune: delete the assets of the collection that are no longer in the source (default false). |
| `source` | object (required) |  | Source of the files: every file becomes an asset, named after its path (subfolders flattened with "-"). |
| `source.configMap` | object |  | ConfigMap: every key of the ConfigMap is a file (data and binaryData). Handy for small applications: a ConfigMap and its Secret are limited to 1 MiB. Keys cannot contain "/", so a key "assets__app.js" becomes the file "assets/app.js". |
| `source.configMap.name` | string (required) |  | Name of the ConfigMap. |
| `source.image` | object |  | Image: an OCI image (a "bundle image") that carries the files. The operator reads its layers directly from the registry: nothing runs in the cluster. Use a digest (repo@sha256:...) for immutable deployments. |
| `source.image.path` | string | `/app` | Path inside the image where the files are (default /app). |
| `source.image.pullSecrets` | []object |  | PullSecrets: Secrets of type kubernetes.io/dockerconfigjson for a private registry. |
| `source.image.pullSecrets[].name` | string (required) |  | Name of the object. |
| `source.image.ref` | string (required) |  | Reference of the image, for example ghcr.io/acme/my-app:1.2.0 or ghcr.io/acme/my-app@sha256:... |
| `source.url` | object |  | URL: a .tar.gz, .tgz or .zip archive downloaded over HTTP(S), for example a CI artifact or a GitHub release asset. |
| `source.url.path` | string |  | Path inside the archive where the files are (default: the root). |
| `source.url.sha256` | string |  | SHA256 of the archive (hex). Recommended: the download is rejected if it does not match, and the operator only downloads it again when this value changes. |
| `source.url.url` | string (required) |  | URL of a .tar.gz, .tgz or .zip archive. |
| `suspend` | boolean |  | Suspend: stop reconciling. |

### status

| Field | Type | Default | Description |
|---|---|---|---|
| `assets` | []object |  | Assets of the collection managed by the operator. |
| `assets[].id` | string (required) |  | ID of the asset in the DAM. |
| `assets[].name` | string (required) |  | Name of the asset (the file name). |
| `assets[].sha256` | string (required) |  | SHA256 of the uploaded file. |
| `assets[].url` | string |  | URL of its original rendition, relative to the DX host (usable in pages and Script Applications). |
| `collectionId` | string |  | CollectionID in the DAM. |
| `conditions` | []object |  |  |
| `conditions[].lastTransitionTime` | string (required) |  | lastTransitionTime is the last time the condition transitioned from one status to another. This should be when the underlying condition changed. If that is not known, then using the time when the API field changed is acceptable. |
| `conditions[].message` | string (required) |  | message is a human readable message indicating details about the transition. This may be an empty string. |
| `conditions[].observedGeneration` | integer |  | observedGeneration represents the .metadata.generation that the condition was set based upon. For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date with respect to the current state of the instance. |
| `conditions[].reason` | string (required) |  | reason contains a programmatic identifier indicating the reason for the condition's last transition. Producers of specific condition types may define expected values and meanings for this field, and whether the values are considered a guaranteed API. The value should be a CamelCase string. This field may not be empty. |
| `conditions[].status` | True \| False \| Unknown (required) |  | status of the condition, one of True, False, Unknown. |
| `conditions[].type` | string (required) |  | type of condition in CamelCase or in foo.example.com/CamelCase. |
| `lastSyncTime` | string |  |  |
| `observedGeneration` | integer |  |  |
| `sourceRevision` | string |  | SourceRevision that was uploaded. |

## DXDatabase

DXDatabase is a database for the backend of an application (PostgreSQL, Oracle or SQL Server), created and kept by the operator.

Short names: `dxdb`

### spec

| Field | Type | Default | Description |
|---|---|---|---|
| `adminSecret` | object (required) |  | AdminSecret: Secret with "username" and "password" of an administrator that can create users and databases (PostgreSQL: a superuser or CREATEROLE + CREATEDB; Oracle: CREATE USER and GRANT; SQL Server: sysadmin or dbcreator + securityadmin). In another namespace only if the operator may read it there (adminSecretNamespace). |
| `adminSecret.name` | string (required) |  | Name of the object. |
| `adminSecretNamespace` | string |  | AdminSecretNamespace: namespace of the admin Secret (default: the namespace of the resource). |
| `database` | string |  | Database name (default: the name of the resource, with "-" replaced by "_"). With Oracle it is the schema: a user with that name. |
| `deletionPolicy` | Retain \| Delete | `Retain` | DeletionPolicy: Retain (default) keeps the database and the user when the resource is deleted; Delete drops them (the data is lost). |
| `engine` | PostgreSQL \| Oracle \| SQLServer | `PostgreSQL` | Engine of the server: PostgreSQL (default), Oracle or SQLServer. |
| `extensions` | []string |  | Extensions (PostgreSQL only) to create in the database, for example [pgcrypto, vector]. |
| `host` | string (required) |  | Host of the database server, for example dx-pg-rw.dx-shared.svc.cluster.local. |
| `oracle` | object |  | Oracle settings (required with engine Oracle). |
| `oracle.service` | string (required) |  | Service name of the pluggable database to connect to, for example FREEPDB1 or ORCLPDB1. |
| `oracle.tablespace` | string | `USERS` | Tablespace for the quota of the user (default USERS). |
| `owner` | string |  | Owner: the user that owns the database and that the backend signs in with (default: the database name). With Oracle it is always the database (schema) name. |
| `port` | integer |  | Port (default: 5432 PostgreSQL, 1521 Oracle, 1433 SQL Server). |
| `secretName` | string |  | SecretName: Secret the operator writes for the backend, with engine, username, password, host, port, database, uri and jdbcUrl. Default: <name>-db. |
| `sqlServer` | object |  | SQLServer settings (engine SQLServer). |
| `sqlServer.encrypt` | disable \| optional \| mandatory \| strict | `mandatory` | Encrypt (TLS of the connection): disable, optional, mandatory (default) or strict (TDS 8). |
| `sqlServer.trustServerCertificate` | boolean |  | TrustServerCertificate: accept the certificate of the server without checking it (self-signed servers in labs). |
| `sslMode` | disable \| prefer \| require \| verify-ca \| verify-full |  | SSLMode (PostgreSQL only): disable, prefer (default), require, verify-ca or verify-full. |

### status

| Field | Type | Default | Description |
|---|---|---|---|
| `conditions` | []object |  |  |
| `conditions[].lastTransitionTime` | string (required) |  | lastTransitionTime is the last time the condition transitioned from one status to another. This should be when the underlying condition changed. If that is not known, then using the time when the API field changed is acceptable. |
| `conditions[].message` | string (required) |  | message is a human readable message indicating details about the transition. This may be an empty string. |
| `conditions[].observedGeneration` | integer |  | observedGeneration represents the .metadata.generation that the condition was set based upon. For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date with respect to the current state of the instance. |
| `conditions[].reason` | string (required) |  | reason contains a programmatic identifier indicating the reason for the condition's last transition. Producers of specific condition types may define expected values and meanings for this field, and whether the values are considered a guaranteed API. The value should be a CamelCase string. This field may not be empty. |
| `conditions[].status` | True \| False \| Unknown (required) |  | status of the condition, one of True, False, Unknown. |
| `conditions[].type` | string (required) |  | type of condition in CamelCase or in foo.example.com/CamelCase. |
| `lastCheckTime` | string |  |  |
| `observedGeneration` | integer |  |  |
| `secretName` | string |  | SecretName written for the backend. |

