# Cómo funciona el operador

[English](how-it-works.md) · [README](../README.es.md)

## Las piezas

- **Una sola imagen**: la imagen pública de DXClient de HCL más el binario del operador. DXClient se ejecuta
  dentro del operador para los push; no se crean Jobs ni imágenes adicionales.
- **Los orígenes** los lee el propio operador: imágenes OCI directamente del registro (sus capas, con
  `go-containerregistry`; no se ejecuta nada), ConfigMaps o archivos por HTTP(S). Se guardan en caché por
  revisión (digest de la imagen, hash del ConfigMap, SHA256 del archivo).
- **APIs de DX**: Ring API (inicio de sesión, contenido de WCM por ID y por nombre), WCM REST v2 (borrado),
  endpoint de XMLAccess `/wps/config` (páginas), API REST del DAM (colecciones, assets, renditions) y la API de
  acceso de Ring (permisos del DAM).

## El bucle de reconciliación

Cada recurso se reconcilia cuando cambia, cuando cambia un objeto al que hace referencia (su ConfigMap, la
colección del DAM de una sustitución, el Deployment del que depende, su conexión...) y cada `resyncInterval` de su
DXConnection (10 minutos por defecto). En cada pasada:

1. **Borrado** (si el recurso se está borrando): con `deletionPolicy: Delete`, quita el objeto de DX; después
   libera el finalizer. `Retain` (por defecto) deja DX como está.
2. **Suspend**: con `spec.suspend: true` se detiene aquí.
3. **Dependencias**: cada `dependsOn` y las implícitas (las colecciones del DAM de las sustituciones, las Script
   Applications de una página). Si alguna no está lista: `Ready=False, reason Waiting`, y se reintenta en 20 s.
4. **Conexión**: la DXConnection debe estar Ready.
5. **Estado deseado**: lo que declara el recurso, como un hash.
6. **Estado real** en DX, y la decisión: crear, actualizar, restaurar, avisar o nada.
7. **Status y eventos**: condiciones `Ready`, `Drifted` y `DependenciesReady`, y eventos de Kubernetes.

Las escrituras en el status son merge patches, así que dos pasadas simultáneas no pierden lo hecho en DX.

### ScriptApplication

- **Preparar**: obtener el origen; copiarlo a una carpeta de trabajo; sustituir los `${NOMBRE}` de
  `substitutions` en los ficheros de texto (`.html .htm .js .mjs .css .json .txt .svg .xml`; los demás `${...}`
  se dejan tal cual); comprobar que existe el HTML principal y que ningún fichero pasa de 4,5 MB.
- **Hash** de todos los ficheros preparados más los ajustes de WCM (nombre, título, site area, fichero principal,
  proyecto) y la anotación de forzado.
- **Decidir**:
  - sin content ID todavía → push (`Created`);
  - el hash cambió → push (`Updated`);
  - si no, leer el contenido en DX por ID (Ring API): 404 → se **borró** en DX; una fecha `updated` distinta de
    la registrada tras el último push → se **modificó** en DX. Con `driftPolicy: Restore` se vuelve a subir
    (`Restored`; tras un borrado WCM crea un elemento nuevo, con content ID nuevo); con `Report` se marca
    `Drifted` y no se toca.
- **Push**: `dxclient deploy-scriptapplication push`, con la conexión en el fichero de configuración de DXClient
  (que se borra después). Solo se da por bueno si DXClient termina con 0 **y** la respuesta de DX tiene
  `"status":"success"` (DXClient dice «successful» aunque DX rechace el contenido). Los errores temporales de un
  DX que está arrancando (Service Unavailable, Bad Gateway, conexión cortada...) se reintentan 5 veces, cada 20 s.
- **Registrar** el content ID, el hash, la revisión del origen, la fecha `updated`, el tamaño y un historial de
  los 10 últimos push.
- **Modo Verify**: nunca sube nada; busca el contenido por su nombre de WCM e informa Ready o `NotFound`.
- **Borrar** (política Delete): WCM REST v2 `DELETE /wcmrest-v2/contents/{id}`. No se usa
  `undeploy-scriptapplication` de DXClient porque se niega mientras una página borrada siga referenciando la
  aplicación (DX elimina las páginas borradas más tarde, con su tarea de limpieza).

### DXPage

- **Deseado**: los content ID de sus Script Applications (en orden), títulos, padre, nombre amigable, plantilla de
  layout, skin, tema, acceso y posición, como un hash. Un segundo hash cubre solo la lista de aplicaciones (el
  layout).
- **Real**: un export de XMLAccess por nombre único. `EJPXA0240E` significa que la página no existe; cualquier
  otro error es un error (un fallo temporal no debe tomarse por una página borrada, o se crearía dos veces). El
  hash aplicado por el operador se guarda en la propia página, como el parámetro `dxop.hash`.
- **Decidir**: no existe y nunca se creó → crearla con su layout; no existe tras haberla creado → se borró en DX
  (restaurar o avisar); cambió la lista de aplicaciones → borrarla y crearla de nuevo (XMLAccess no puede
  sustituir los componentes de una página); cambió otro ajuste → actualizar títulos, parámetros y acceso sin
  tocar el layout (los ajustes que la gente hace en la página se conservan); el `dxop.hash` de DX es distinto
  del aplicado → alguien cambió los ajustes en DX (restaurar o avisar).
- Una Script Application que recibe un content ID nuevo (restaurada tras un borrado) dispara sus páginas: cambia
  el hash del layout y la página se reconstruye.

### DXTheme

- **Deseado**: el spec (carpeta, nombre único, títulos, base, HTML de cabecera) y la revisión de su origen, como un hash.
- **Real**: un export de XMLAccess del tema por nombre único (el hash aplicado se guarda en el tema, en el parámetro
  `dxop.hash`) y un listado WebDAV de su carpeta (`dav:fs-type1/themes/<carpeta>/`).
- **Aplicar**: descargar todos los ficheros del tema base por WebDAV, poner encima los del origen (mismas rutas),
  insertar el HTML de cabecera antes de `</head>` en `theme.html`, `theme_sidenav.html` y sus versiones por idioma,
  subir la carpeta (MKCOL + PUT) y registrar el tema con una copia del registro del tema base (nombre único nuevo,
  plantillas apuntando a la carpeta nueva, `default="false"`, solo los títulos declarados).
- **Deriva**: falta el registro o la carpeta en DX → subirlo y registrarlo otra vez (`Restore`) o avisar.
- **Borrar** (política Delete): borrar el registro (XMLAccess) y la carpeta (WebDAV). Las páginas que lo usan pasan
  al tema por defecto.
- WebDAV y XMLAccess usan autenticación básica: el tema funciona igual con DX detrás de OpenID Connect.
- No se usa `deploy-theme` de DXClient: su paso de registro falla en DX Compose CF239.

### DAMCollection

- Buscar la colección por el ID del status, o por nombre (así se adopta una existente), o crearla; dar el rol User
  a los anónimos y/o a todos los usuarios autenticados.
- Para cada fichero del origen (assets con el nombre de su ruta, `/` → `-`): sin cambios (mismo SHA256 registrado)
  → nada; cambiado → `PUT` de una versión nueva del mismo asset (mismo ID y URL); no está en DX → subirlo de nuevo
  (restaurar o avisar); ya hay un asset con ese nombre → adoptarlo. Después esperar a su rendition original y
  registrar su URL.
- `prune: true` borra los assets que ya no están en el origen.
- Borrar (política Delete): la colección, ignorando las referencias.

### PostgresDatabase

- Un Secret `<nombre>-db` (o `secretName`) con los datos de conexión; la contraseña se genera una vez y se
  conserva, y el Secret pertenece al recurso.
- Con el Secret del superusuario: crear o modificar el rol (login, contraseña), crear la base de datos con ese
  propietario si falta y crear las extensiones. Se revisa cada 30 minutos.
- Borrar (política Delete): `DROP DATABASE ... WITH (FORCE)` y `DROP ROLE`.

### DXConnection

Inicia sesión en DX con una sesión nueva (`/dx/api/core/v1/auth/login`) y comprueba que DXClient puede ejecutarse
(licencia aceptada, ficheros presentes). Un cambio en sus Secrets dispara una nueva comprobación.

## Concurrencia y rendimiento

- Una réplica activa (leader election). Las ejecuciones de DXClient se **serializan** (de una en una): DXClient
  guarda sus ajustes en su propia carpeta, y DX tampoco lleva bien los push en paralelo. Las llamadas REST van en
  paralelo.
- Un push tarda unos segundos; una página, alrededor de un segundo; una revisión (resync), una o dos llamadas REST
  por recurso.
- Las sesiones REST se reutilizan por conexión mientras no cambien sus Secrets.

## Forzar y pausar

- `metadata.annotations["dx.europesip.com/reconcile"]`: cambia su valor para reconciliar al momento; en una
  Script Application fuerza además un push (el valor forma parte del hash).
- `spec.suspend: true`: no se hace nada hasta que vuelva a ser false.

## Status y eventos

```bash
kubectl get scriptapplication vacaciones -o yaml        # status.contentId, history, conditions
kubectl get events --field-selector involvedObject.name=vacaciones
```

Eventos: `Pushed` (Created / Updated / Restored), `Drifted`, `PushFailed`, `Created`, `Rebuilt`, `Synced`,
`Deleted`, `Undeployed`, `Dropped`, `Connected`, `LoginFailed`...
