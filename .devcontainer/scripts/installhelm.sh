#! /bin/bash 
set -e
set -x

# Install helm 3.0
curl https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 | DESIRED_VERSION=v3.0.3 bash

# add the stable chart repo
helm repo add stable https://kubernetes-charts.storage.googleapis.com/
helm repo update