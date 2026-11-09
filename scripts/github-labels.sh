#!/usr/bin/env bash
# scripts/github-labels.sh · creates (or updates) the labels of the work model described in docs/workflow.md.
# Usage: scripts/github-labels.sh [owner/repo]   (default: europesip/hcl-dx-scriptapp-operator). Needs gh with repo scope.
set -euo pipefail
R="${1:-europesip/hcl-dx-scriptapp-operator}"
l(){ gh label create "$1" -R "$R" --color "$2" --description "$3" --force; }
# Type
l bug            CF222E "Something does not behave as it should"
l tech-debt      E36209 "A shortcut taken on purpose, to be paid back"
l enhancement    1A7F37 "A new capability, option or use case"
l blocked        6F42C1 "Depends on another task or on something external"
l documentation  0075CA "Documentation or architecture"
# Area
l area:controller  0969DA "Area: reconcilers, status and events"
l area:crd         BF8700 "Area: custom resources and their API"
l area:dx-api      BF3989 "Area: DX access (DXClient, WCM, XMLAccess, DAM)"
l area:database    0E8A8A "Area: DXDatabase (PostgreSQL, Oracle, SQL Server)"
l area:packaging   5B6CFF "Area: Helm chart, image, CI and samples"
# Priority
l P1 B60205 "High priority"
l P2 FBCA04 "Medium priority"
l P3 A2EEEF "Low priority"
