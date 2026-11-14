# How we work: issues, board and specs

<p><b>English</b> · <a href="workflow.es.md">Español</a></p>

Every change to HCL DX Script Application operator starts as an **issue**, is tracked on the [project board](https://github.com/orgs/europesip/projects/3), and is closed by a
**pull request** that proves its acceptance criteria. Each change is defined before it is built, documented in one
place, and easy to follow: the first step towards **spec-driven development**.

## 1 · Open an issue (the spec)

| Form | Label | Use it for |
|---|---|---|
| 🚀 Enhancement (spec) | `enhancement` | A new capability, option or use case |
| 🐞 Bug | `bug` | Something does not behave as it should |
| 🧱 Tech debt | `tech-debt` | A shortcut to pay back before it becomes a bug |

Blank issues are disabled, so everyone writes them the same way. The key part is the **acceptance criteria**:
checkable statements ("given … when … then …"). Big changes are **epics**: split them into sub-issues.

## 2 · Triage

| Field | Values |
|---|---|
| **Priority** (label) | 🔴 `P1` high · 🟡 `P2` medium · 🔵 `P3` low |
| **Area** (label) | `area:controller` · `area:crd` · `area:dx-api` · `area:database` · `area:packaging` |
| **Effort** (number) | Relative scale 1-2-3-5-8-13 (1 ≈ one hour, 13 = an epic to split before starting) |
| **Release** | *0.3.x* · *0.4.0* · *0.5.0* · *Later* |

`blocked` marks an issue that depends on another task or on something external.

## 3 · The board

| Column | Meaning |
|---|---|
| 📋 **Backlog** | Defined and waiting to start |
| 🚧 **In progress** | Active work (2 or 3 at most at a time) |
| 🧪 **In testing / Blocked** | Being tested on the lab cluster, or stopped by a dependency |
| ✅ **Done** | Finished and checked |

Views: full table, Kanban, board by release (with and without the finished items), and one table each for tech
debt, enhancements and bugs. Insights: current status, work over time and load per release.

## 4 · Build it

1. Move the issue to **In progress** and create a branch `<number>-<short-name>`.
2. Open a pull request with `Closes #<number>`; its template asks for the acceptance criteria, the tests and the
   release checklist.
3. While it is tested, the issue is **In testing**. When the pull request is merged, it closes and moves to **Done**.

## 5 · Towards spec-driven development

For changes with an effort of 5 or more, the spec also lives in the repository: `docs/specs/<number>-<short-name>.md`,
from [docs/specs/TEMPLATE.md](https://github.com/europesip/hcl-dx-scriptapp-operator/blob/main/docs/specs/TEMPLATE.md). It is reviewed and merged (*Approved*) **before** the
implementation, and the pull request that closes the issue marks it *Implemented*. A versioned spec is the input of
the implementation, whether a person or an AI coding agent writes the code, and the reference for its review.
