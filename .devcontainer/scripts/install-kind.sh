#! /bin/bash 
set -e
set -x

curl -Lo ./kind https://github.com/kubernetes-sigs/kind/releases/download/v0.5.1/kind-linux-amd64
chmod +x ./kind
mv ./kind /usr/local/bin/kind
