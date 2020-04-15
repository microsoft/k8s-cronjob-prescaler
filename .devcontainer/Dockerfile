#-------------------------------------------------------------------------------------------------------------
# Copyright (c) Microsoft Corporation. All rights reserved.
# Licensed under the MIT License. See https://go.microsoft.com/fwlink/?linkid=2090316 for license information.
#-------------------------------------------------------------------------------------------------------------

FROM golang:1.12-stretch

# Avoid warnings by switching to noninteractive
ENV DEBIAN_FRONTEND=noninteractive

# Configure apt, install packages and tools
RUN apt-get update \
    && apt-get -y install --no-install-recommends apt-utils dialog nano bash-completion 2>&1 \
    #
    # Verify git, process tools, lsb-release (common in install instructions for CLIs) installed
    && apt-get -y install git iproute2 procps lsb-release \
    # Install docker
    && apt-get install -y apt-transport-https ca-certificates curl gnupg-agent software-properties-common lsb-release \
    && curl -fsSL https://download.docker.com/linux/$(lsb_release -is | tr '[:upper:]' '[:lower:]')/gpg | apt-key add - 2>/dev/null \
    && add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/$(lsb_release -is | tr '[:upper:]' '[:lower:]') $(lsb_release -cs) stable" \
    && apt-get update \
    && apt-get install -y docker-ce-cli \
    # Install icu-devtools
    && apt-get install -y icu-devtools \ 
    # Install kubectl
    && curl -sSL -o /usr/local/bin/kubectl https://storage.googleapis.com/kubernetes-release/release/v1.16.2/bin/linux/amd64/kubectl \
    && chmod +x /usr/local/bin/kubectl \
    # Install jq
    && apt-get install -y jq \
    # Install Azure cli
    && curl -sL https://aka.ms/InstallAzureCLIDeb | bash - \
    # Clean up
    && apt-get autoremove -y \
    && apt-get clean -y \
    && rm -rf /var/lib/apt/lists/*

# Enable go modules
ENV GO111MODULE=on

# Install Go tools
RUN \
    # --> Delve for debugging
    go get github.com/go-delve/delve/cmd/dlv@v1.3.2 \
    # --> Go language server
    && go get golang.org/x/tools/gopls@v0.2.1 \
    # --> Go symbols and outline for go to symbol support and test support 
    && go get github.com/acroca/go-symbols@v0.1.1 && go get github.com/ramya-rao-a/go-outline@7182a932836a71948db4a81991a494751eccfe77 \
    # --> GolangCI-lint
    && curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sed 's/tar -/tar --no-same-owner -/g' | sh -s -- -b $(go env GOPATH)/bin \
    # --> Install Ginkgo
    && go get github.com/onsi/ginkgo/ginkgo@v1.12.0  \
    # --> Install junit converter
    && go get github.com/jstemmer/go-junit-report@v0.9.1 \
    && rm -rf /go/src/ && rm -rf /go/pkg

# Install other stuff using scripts because it's nicer
COPY ./scripts/kubebuilder.sh .
RUN bash -f ./kubebuilder.sh

COPY ./scripts/kustomize.sh .
RUN bash -f ./kustomize.sh

COPY ./scripts/installhelm.sh .
RUN bash -f ./installhelm.sh

COPY ./scripts/install-kind.sh .
RUN bash -f ./install-kind.sh

COPY ./scripts/configbash.sh .
RUN bash -f ./configbash.sh

# Install golangci-linter
RUN curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh| sh -s -- -b $(go env GOPATH)/bin v1.21.0

# Build the path
ENV PATH="/usr/local/kubebuilder/bin:${PATH}"
ENV PATH="/root/.kubectx:${PATH}"

# Set Kubeconfig path to kind cluster
ENV KUBECONFIG="/root/.kube/kind-config-psccontroller"
ENV KUBE_EDITOR="nano"