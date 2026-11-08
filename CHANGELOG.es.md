# Historial de versiones

[English](CHANGELOG.md) · **Español**

Para volver a una versión anterior: `helm upgrade dxop oci://ghcr.io/europesip/charts/hcl-dx-scriptapp-operator --version <versión> --reuse-values`
(la 0.3.0 renombró los recursos: mira sus notas).

## 0.3.0 · 2026-11-08 · Prefijo DX, DXDatabase para cualquier motor, temas propios

**Cambio incompatible: los recursos se han renombrado** para que todos lleven el prefijo DX:

| Antes | Ahora | Nombre corto |
|---|---|---|
| `ScriptApplication` | `DXScriptApplication` | `dxapp`, `dxsa` |
| `DAMCollection` | `DXDamCollection` | `dxdam` |
| `PostgresDatabase` | `DXDatabase` (`superuserSecret` → `adminSecret`) | `dxdb` |

`DXConnection`, `DXPage` y `DXTheme` mantienen su nombre. **Migrar desde la 0.2:** aplica los CRDs de la 0.3.0, cambia el
`kind` de tus recursos (y `superuserSecret` / `superuserSecretNamespace` → `adminSecret` / `adminSecretNamespace`) y
vuelve a aplicarlos: el operador encuentra el contenido existente en DX por su nombre (nombre en WCM, nombre de la
colección, base de datos) y lo adopta. Después borra los recursos antiguos (sus finalizers solo los quita un operador
que los conozca: con la 0.3.0 en marcha, quítalos con `kubectl patch ... --type merge -p '{"metadata":{"finalizers":null}}'`;
no se borra nada en DX) y los CRDs antiguos.

- **DXDatabase**: PostgreSQL, **Oracle** (`oracle.service`; la base de datos es un esquema) y **SQL Server**
  (`sqlServer.encrypt`, `trustServerCertificate`), con `engine`. El mismo Secret con cualquier motor (`engine`,
  `username`, `password`, `host`, `port`, `database`, `uri`, `jdbcUrl`). Probado con PostgreSQL 17, Oracle Database 23ai
  Free y SQL Server 2022: crear, entrar como propietario, volver a reconciliar y borrar. DB2 aún no está soportado (su
  driver de Go necesita cgo).
- **DXTheme**: `base.copyFiles: false` sube solo los ficheros del origen (un tema propio, sin copiar nada de los temas de
  HCL); `profile` y `layout` fijan el perfil de agregación de recursos y el layout por defecto. El ejemplo 09 instala
  ahora el [tema de ejemplo de EUROPESIP](https://github.com/europesip/hcl-dx-theme-sample) desde su release.

 2026-11-08 · Temas

(La 0.2.0 se etiquetó pero nunca se publicó: su CI falló por la cabecera de un fichero generado.)

- Recurso nuevo **DXTheme** (nombre corto `dxtheme`): un tema WebDAV propio. El operador copia un tema base del mismo
  DX (por defecto Portal 8.5) por WebDAV, añade los ficheros de su `source` (ConfigMap, imagen o URL, como las Script
  Applications) y el HTML de `headHTML` en el `<head>` de sus plantillas, sube la carpeta y registra el tema con
  XMLAccess (copia del registro del tema base). La deriva (registro o carpeta borrados en DX) se restaura o se
  avisa; `deletionPolicy: Delete` borra el registro y la carpeta.
- Las páginas lo usan con `spec.theme` (su nombre único, por defecto `dxop.<namespace>.<nombre>`) y pueden esperarlo
  con `dependsOn: [{kind: DXTheme, name: ...}]` (tipo de dependencia nuevo).
- Ejemplo nuevo [09-theme](samples/09-theme/theme.yaml) y health check de ArgoCD para DXTheme.
- Funciona con y sin OpenID Connect (autenticación básica para WebDAV y XMLAccess). Probado con DX Compose CF239,
  con y sin Keycloak.
- **Corrección:** tras un reinicio de DX (por ejemplo un `helm upgrade` de DX) la cookie SSO antigua seguía en el
  operador, el nuevo login se saltaba el login por formulario y todas las llamadas respondían 401 hasta reiniciar el
  operador (recursos en `DXError`). Ahora cada login empieza con el almacén de cookies vacío.
- CRD nuevo: Helm solo instala los CRDs en la primera instalación; con una release existente, aplica antes
  `charts/hcl-dx-scriptapp-operator/crds/`. Los demás CRDs solo ganan el tipo de dependencia `DXTheme`.

## 0.1.3 · 2026-11-08 · Un interruptor para la compatibilidad con OpenID Connect

- Nuevo valor del chart **`dx.oidcCompatibility`** (por defecto `true`). Con `false` se desactiva el login de formulario
  de la 0.1.2 y se vuelve al comportamiento de la 0.1.1: solo el login REST de DX. Fija la variable de entorno
  `DX_FORM_LOGIN` (`auto` / `false`) del operador.

## 0.1.2 · 2026-11-08 · DX con OpenID Connect (Keycloak, Entra ID...)

- **Qué cambia:** cuando el login REST de DX (`/dx/api/core/v1/auth/login`) responde sin cookie de sesión, el operador
  entra con el formulario de login del portal (`<context root>/j_security_check`, sin seguir la redirección) y usa esa
  sesión para las llamadas a la Ring API y al DAM.
- **Por qué:** cuando DX autentica a los usuarios con OpenID Connect (la feature `openidConnectClient-1.0` de Liberty),
  Liberty llama a la cookie de SSO `WAS_n<hash>` en lugar de `LtpaToken2`, y no se puede configurar (issue 18803 de Open
  Liberty). El login REST de DX responde entonces 200 sin cookie, y todas las llamadas REST del operador daban 401: los
  recursos se quedaban en `DXError` y la DXConnection nunca estaba Ready. La Ring API y la API del DAM aceptan la cookie
  nueva. Probado con DX Compose CF239 y Keycloak 26.8.
- **En DX:** el `authFilter` de OIDC debe dejar fuera `/wps/config` (XMLAccess, como documenta HCL), `j_security_check` y
  las peticiones con `Authorization: Basic` (DXClient). Ver [docs/limitations.es.md](docs/limitations.es.md).
- **Sin OIDC no cambia nada:** el login REST devuelve `LtpaToken2` y no se hace login de formulario.
- **Volver atrás:** chart 0.1.3 con `dx.oidcCompatibility=false`, o `--version 0.1.1`.

## 0.1.1 · 2026-11-08 · Páginas adoptadas

- Una DXPage que ya estaba en DX tal como se declaraba (adoptada) registra ahora su estado aplicado. Antes, una página
  adoptada ignoraba `deletionPolicy: Delete` (se quedaba en DX al borrar el recurso) y los cambios de layout posteriores.

## 0.1.0 · 2026-11-08 · Primera versión

- ScriptApplication, DXPage, DAMCollection, PostgresDatabase y DXConnection: Script Applications subidas con HCL DXClient
  (dentro de la imagen del operador), páginas con XMLAccess, ficheros del DAM, bases de datos de los backends, detección y
  restauración de la deriva, health checks de Argo CD.
