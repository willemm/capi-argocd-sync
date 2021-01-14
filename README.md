## Kubernetes Cluster-API to ArgoCD clusters

There is a management cluster that runs Cluster-API to create and manage K8S clusters,
This cluster uses GitOps (i.e. ArgoCD) to manage all the cluster manifests,
Also, this cluster deploys packages on the created clusters.

To make this possible, these newly created clusters should be automatically added to the ArgoCD config.

Fortunately, this is quite easy: Cluster-API creates a secret <clustername>-kubeconfig for each cluster it creates,
which contains the configuration and credentials necessary to connect to the cluster.
It just needs to be translated to the format of an argocd cluster-secret.

That's what this controller does, basically.

Note: I just started this, it seems to be working, but not really tested or anything yet.
Just putting it out here, but I don't plan on supporting it or anything.
