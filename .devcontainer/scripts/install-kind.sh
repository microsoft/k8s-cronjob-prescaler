#! /bin/bash 
set -e
set -x

curl -Lo ./kind https://github.com/kubernetes-sigs/kind/releases/download/v0.7.0/kind-linux-amd64
chmod +x ./kind
mv ./kind /usr/local/bin/kind
