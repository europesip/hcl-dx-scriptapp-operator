# Cómo trabajamos: issues, tablero y especificaciones

<p><a href="workflow.md">English</a> · <b>Español</b></p>

Cada cambio de HCL DX Script Application operator empieza como un **issue**, se sigue en el [tablero del proyecto](https://github.com/orgs/europesip/projects/3) y se cierra con un
**pull request** que demuestra sus criterios de aceptación. Cada cambio se define antes de construirse, se documenta
en un único sitio y es fácil de seguir: el primer paso hacia el **desarrollo guiado por especificaciones**.

## 1 · Abrir un issue (la especificación)

| Formulario | Etiqueta | Para |
|---|---|---|
| 🚀 Enhancement (spec) | `enhancement` | Una capacidad, opción o caso de uso nuevo |
| 🐞 Bug | `bug` | Algo no se comporta como debería |
| 🧱 Tech debt | `tech-debt` | Un atajo que hay que pagar antes de que se convierta en un bug |

Los issues en blanco están desactivados, para que todos se escriban igual. Lo importante son los **criterios de
aceptación**: afirmaciones comprobables («dado … cuando … entonces …»). Los cambios grandes son **épicas**: se dividen
en sub-issues.

## 2 · Clasificación (triage)

| Campo | Valores |
|---|---|
| **Prioridad** (etiqueta) | 🔴 `P1` alta · 🟡 `P2` media · 🔵 `P3` baja |
| **Área** (etiqueta) | `area:controller` · `area:crd` · `area:dx-api` · `area:database` · `area:packaging` |
| **Effort** (número) | Escala relativa 1-2-3-5-8-13 (1 ≈ una hora, 13 = una épica que se divide antes de empezar) |
| **Release** | *0.3.x* · *0.4.0* · *0.5.0* · *Later* |

`blocked` marca un issue que depende de otra tarea o de algo externo.

## 3 · El tablero

| Columna | Significado |
|---|---|
| 📋 **Backlog** | Definido y pendiente de empezar |
| 🚧 **In progress** | Trabajo activo (2 o 3 como máximo a la vez) |
| 🧪 **In testing / Blocked** | Probándose en el clúster del laboratorio, o detenido por una dependencia |
| ✅ **Done** | Terminado y comprobado |

Vistas: tabla completa, Kanban, tablero por release (con y sin lo terminado) y una tabla para la deuda técnica, otra
para las mejoras y otra para los bugs. Gráficos: estado actual, evolución del trabajo y carga por release.

## 4 · Construirlo

1. Mueve el issue a **In progress** y crea una rama `<número>-<nombre-corto>`.
2. Abre un pull request con `Closes #<número>`; su plantilla pide los criterios de aceptación, las pruebas y la lista
   de la release.
3. Mientras se prueba, el issue está **In testing**. Al fusionar el pull request, se cierra y pasa a **Done**.

## 5 · Hacia el desarrollo guiado por especificaciones

Para los cambios con un esfuerzo de 5 o más, la especificación vive también en el repositorio:
`docs/specs/<número>-<nombre-corto>.md`, a partir de [docs/specs/TEMPLATE.md](https://github.com/europesip/hcl-dx-scriptapp-operator/blob/main/docs/specs/TEMPLATE.md). Se revisa y se fusiona
(*Approved*) **antes** de la implementación, y el pull request que cierra el issue la marca como *Implemented*. Una
especificación versionada es la entrada de la implementación, la escriba una persona o un agente de IA, y la
referencia de su revisión.
