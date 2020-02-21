#! /bin/bash 
set -e
set -x

# download kustomize
curl -o /tmp/kustomize -sL "https://github.com/kubernetes-sigs/kustomize/releases/download/v3.1.0/kustomize_3.1.0_linux_amd64"

cp /tmp/kustomize /usr/local/kubebuilder/bin/ 

# set permission
chmod a+x /usr/local/kubebuilder/bin/kustomize