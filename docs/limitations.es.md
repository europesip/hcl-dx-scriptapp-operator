# Restricciones y limitaciones

[English](limitations.md) · [README](../README.es.md)

## Script Applications

- **El HTML principal debe ser un documento HTML completo** (`<!DOCTYPE html><html><head>…</head><body>…</body></html>`).
  DXClient conserva los elementos `<link>` y `<script>` del `<head>` (reescribiendo sus URL a los ficheros que
  guardó en WCM) y el contenido del `<body>`. Con un fragmento suelto se pierden el elemento exterior y los
  `<link>`. Las compilaciones de Vite, webpack o `create-dx-script-app` de HCL ya generan documentos completos.
- **Scripts**: se ejecutan tanto los scripts en línea como los ficheros enlazados desde el `<head>`. Los del
  `<head>` se ejecutan antes de que exista el marcado del body: usa `defer`, o espera a tu elemento raíz.
- **Varias instancias en una página** comparten la página: selecciona tu elemento raíz por clase o usa el token
  de namespace de DX en los ID.
- **Cada fichero tiene un límite de 4,5 MB**: WCM guarda cada fichero en un elemento de 5.000.000 bytes como
  máximo, y uno mayor hace que DX rechace el push y deje un elemento incompleto. El operador comprueba los tamaños
  antes de tocar DX. Divide el bundle (code splitting) o sirve los ficheros grandes desde el DAM.
- **El CSS global cambia el estilo de todo el portal**: limita cada regla a tu aplicación.
- **El nombre de WCM es la clave** (`spec.wcm.name`, por defecto el nombre del recurso): cambiarlo crea una
  segunda aplicación; la antigua se queda en DX.
- **Una aplicación restaurada tiene un content ID nuevo** (tras borrarse en DX). Sus páginas se reconstruyen
  solas, con los ajustes por defecto de los visores.
- **Los cambios hechos en DX se sobrescriben** con `driftPolicy: Restore` (por defecto): el operador compara la
  fecha de última modificación del contenido. Usa `Report` donde los autores trabajen la aplicación en DX.
- **Push o sindicación, por entorno**: si un entorno recibe la aplicación por sindicación, un push allí crea un
  segundo elemento distinto. Usa `mode: Verify` en ese entorno.
- **Proyectos de WCM** (`wcm.projectContext`): el proyecto debe existir; publicarlo es cosa tuya.
- **Etiquetas móviles** (`:latest`, `:main`): el operador resuelve el digest en cada pasada, así que una imagen
  nueva con la misma etiqueta se sube en el siguiente resync. Fija un digest (`repo@sha256:...`) para despliegues
  inmutables.
- **Las sustituciones** solo se aplican a los ficheros de texto con las extensiones de
  [cómo funciona](how-it-works.es.md), y sus valores acaban en ficheros que cualquier visitante puede descargar.
- **Los orígenes ConfigMap** tienen un límite de 1 MiB en total, y las claves no pueden contener `/`
  (`assets__app.js` → `assets/app.js`).

## Páginas

- **El usuario de la conexión debe ser administrador de DX** (XMLAccess).
- **Cambios hechos en la interfaz de DX: de momento solo se detecta una página borrada.** Una página borrada en DX,
  desde la interfaz (Site Manager → *Delete Page*) o por script, se vuelve a crear en la siguiente resincronización
  (por defecto cada 5 minutos), con sus títulos y aplicaciones. Una página que sigue existiendo pero que se ha
  **cambiado** en la interfaz (renombrada, URL amigable, aplicaciones quitadas o añadidas, acceso, tema, layout o
  posición) **todavía no** se detecta: el operador compara una huella que guardó en la página, y la interfaz no la
  cambia, así que la página sigue `InSync`. Para volver a aplicar la página declarada, fuerza una reconciliación:
  `kubectl -n <namespace> annotate dxpage <nombre> dx.europesip.com/reconcile="$(date +%s)" --overwrite`.
  Corregirlo (comparar el estado real de la página) es el
  [issue #2](https://github.com/europesip/hcl-dx-scriptapp-operator/issues/2).
- **Cambiar la lista de aplicaciones vuelve a crear la página** (XMLAccess no puede sustituir los componentes de
  una página): se pierden los ajustes de los visores y los cambios de layout. Títulos, acceso, tema, padre y
  posición se actualizan sin recrearla.
- **Páginas anónimas**: `access.anonymous: true` abre la página, pero las Script Applications son contenido de WCM
  y necesitan que los usuarios anónimos tengan el rol User en la **Script Application Library** (se fija una vez
  en DX, *Web Content Libraries* → biblioteca → acceso). El operador no puede fijar permisos de elementos de WCM
  (DX no ofrece una API para ello).
- **DX solo marca las páginas borradas**, y las elimina más tarde con su tarea de limpieza: hasta entonces DX
  sigue viendo referencias a ellas.
- Un solo layout: las aplicaciones se apilan en una columna (la plantilla `1Column` por defecto).
- La URL del status solo se calcula para páginas bajo `ibm.portal.Home`.

## DAM

- Cada fichero del origen se convierte en un asset con el nombre de su ruta (`img/logo.png` → `img-logo.png`).
- Un fichero sustituido en el DAM por alguien (una versión nueva) no se detecta: solo los assets y colecciones
  borrados.
- Acceso a la colección: el operador concede; no retira permisos dados en DX.
- Borrar una colección (política Delete) ignora las referencias: las páginas y aplicaciones que usan sus assets
  muestran enlaces rotos.

## Temas

- **Solo temas WebDAV** (los ficheros del tema en `dav:fs-type1/themes/`). No se soportan temas empaquetados como
  WAR/EAR.
- **El registro toma como modelo un tema del mismo DX** (el base, por defecto Portal 8.5: skins y ajustes), así que el
  base tiene que existir en ese DX. Con `base.copyFiles: true` (por defecto) también se copian sus ficheros, y el tema
  no sigue los cambios posteriores del base: fuerza una reconciliación (o cambia el recurso) para copiarlo otra vez.
  Un tema propio (`base.copyFiles: false`) necesita un origen con todos los ficheros del tema: mira el
  [tema de ejemplo de EUROPESIP](https://github.com/europesip/hcl-dx-theme-sample).
- **La carpeta y el nombre único son la clave**: cambiarlos crea un segundo tema; el anterior se queda en DX (salvo
  con la política Delete).
- **Aplicar un cambio sube la carpeta entera** (unos 430 ficheros con Portal 8.5, uno o dos minutos).
- **No se detectan los cambios hechos en DX a los ficheros del tema**; solo que se borre el registro o la carpeta.
- **Las páginas** eligen el tema con `spec.theme` (su nombre único); para los layouts del tema nuevo, apunta
  `layoutTemplate` a `dav:fs-type1/themes/<carpeta>/layout-templates/...`.

## DXDatabase

- Motores: **PostgreSQL, Oracle y SQL Server** (probados con PostgreSQL 17 / CloudNativePG, Oracle Database 23ai Free
  y SQL Server 2022). **DB2 aún no está soportado**: su driver de Go necesita la librería nativa del cliente de IBM
  (cgo), que la imagen estática del operador no incluye.
- Un servidor existente y un Secret de administrador que el operador pueda leer (puede estar en otro namespace con
  `adminSecretNamespace`). El operador genera la contraseña del propietario; si tu DBA debe custodiar las credenciales,
  no uses DXDatabase y da al backend el Secret que te facilite (por ejemplo, un ExternalSecret de tu gestor).
- Nombres: PostgreSQL, identificadores en minúsculas; Oracle, un esquema en minúsculas de hasta 30 caracteres (el
  usuario se crea en mayúsculas); SQL Server, letras, dígitos y `_`.
- El operador no migra esquemas: eso corresponde al backend (Flyway, Liquibase o su propio arranque).

## Operador

- **No es un producto de HCL** y HCL no le da soporte. DXClient se usa sin modificar, tal como lo distribuye HCL.
- Una réplica activa; las ejecuciones de DXClient se serializan, así que muchos push se ponen en cola.
- Probado con HCL DX Compose CF239 y DXClient `v95_CF239_20261103-1404`. Para usar el DXClient de tu nivel de DX,
  construye la imagen con `--build-arg DXCLIENT_IMAGE=hclcr.io/dx-public/dxclient:<tag>`.
- **DX con OpenID Connect (Keycloak, Entra ID...)**: soportado desde la 0.1.2 (`dx.oidcCompatibility=false` en el chart lo desactiva, 0.1.3). Con el cliente OIDC de Liberty la cookie
  de SSO recibe un nombre generado (`WAS_n...`, `WAS_p...`: la letra depende del DX, en lugar de `LtpaToken2`) y el login REST de DX responde sin cookie, así
  que el operador usa el login de formulario del portal. El `authFilter` OIDC de DX debe dejar fuera `/wps/config`
  (XMLAccess, lo documenta HCL), `j_security_check` y las peticiones con `Authorization: Basic` (DXClient).
- API `v1alpha1`: los campos pueden cambiar antes de `v1`.
