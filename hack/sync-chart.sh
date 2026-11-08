#!/usr/bin/env bash
# Copies the generated CRDs and RBAC rules into the Helm chart (run after "make manifests"; CI checks it).
set -euo pipefail
cd "$(dirname "$0")/.."
C=charts/hcl-dx-scriptapp-operator
rm -f $C/crds/*.yaml
cp config/crd/bases/*.yaml $C/crds/
# The rules of the generated ClusterRole, as is
sed -n '/^rules:/,$p' config/rbac/role.yaml > $C/files/rules.yaml
echo "chart synced: $(ls $C/crds | wc -l) CRDs, $(grep -c apiGroups $C/files/rules.yaml) rules"
