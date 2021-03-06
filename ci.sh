#! /bin/bash

set -e 
set -x 

# Get storage drive details
docker info

# Create .dockercache directory
mkdir -p ./.dockercache/

# Import devcontainer from cache to speed up build
if [ -f ".dockercache/devcontainer.tar" ]; 
then
    echo "-------> Restoring docker image"
    time docker load -i .dockercache/devcontainer.tar
fi

echo "-------> Building devcontainer"
# Use the devcontainer to run the build as it has all the environment setup that we need
time docker build --cache-from devcontainer:latest -t devcontainer -f ./.devcontainer/Dockerfile ./.devcontainer

# Create a directory for go mod cache
mkdir -p ${PWD}/.gocache

echo "-------> Building code and running tests"
# Run `make` to build and test the code
time docker run -v ${PWD}/.gocache:/go/pkg/ -v /var/run/docker.sock:/var/run/docker.sock -v ${PWD}:/src --workdir /src --entrypoint /bin/bash --network="host" devcontainer -c "K8S_NODE_IMAGE=$K8S_NODE_IMAGE make build-run-ci"

# Ensure .gocache permmissions correct for build to save cache
sudo chown -R $USER ${PWD}

# If the current cached image is out of date save devcontainer so it can be cached
if [ $DOCKER_CACHE_HIT != "true" ];
then
    echo "-------> Saving docker image"
    time docker image save -o ./.dockercache/devcontainer.tar devcontainer
fi
# Build and tag initcontainer docker image
echo "-------> Building and tagging initcontainer"
docker build -t initcontainer:latest-${BUILD_BUILDNUMBER} ./initcontainer