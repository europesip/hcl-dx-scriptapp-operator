# GitOps with ArgoCD

1. Health checks, so ArgoCD shows the resources of the operator as Progressing, Healthy, Degraded or Suspended:

   ```bash
   kubectl -n argocd patch configmap argocd-cm --patch-file health.yaml
   ```

2. The namespace of the applications and its Secrets (they do not belong in Git):

   ```bash
   kubectl create namespace dxop-samples
   kubectl -n dxop-samples create secret generic dx-admin --from-literal=username='<DX admin>' --from-literal=password='<password>'
   kubectl -n dxop-samples create secret generic dx-ca --from-file=ca.crt=dx-ca.crt
   ```

3. The Application ([application.yaml](application.yaml)) points at [samples/](../../samples) of this repository:

   ```bash
   kubectl apply -f application.yaml
   ```

ArgoCD applies the resources; the operator pushes the Script Applications, creates the pages and the DAM
collection, prepares the database, and reports everything in the status, which ArgoCD shows as health. A new
version of an application is a commit that changes its image tag or digest; deleting a file from Git deletes
the resource (`prune`), and with `deletionPolicy: Delete` the operator also removes it from DX.

YAML anchors (`&health` / `*health`) keep one copy of the Lua script for the five resources; `kubectl patch`
resolves them.

## Tested

With Argo CD v3.6.1 on the lab cluster (2026-11-08): the Application synced the 9 samples in about a minute,
every resource reported Healthy with the operator's message, and a commit that changed sample 01 was pushed to DX
6 seconds after the sync. In Argo CD 3 the health of each resource is shown in the UI and in the resource tree
(`argocd app resources`, API `/applications/<name>/resource-tree`), not in `status.resources` of the Application.
