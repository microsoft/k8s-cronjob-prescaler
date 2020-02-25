#!/bin/bash
set -e

cd "$(dirname "$0")"

#
# Argument parsing and validation
#

function show_usage() {
    echo "deploy-prometheus.sh (PROMETHEUS_INSTANCE_NAME)"
    echo
    echo "first argument optionally defines the prometheus instance name"
}

function deploy_dashboard() {
    local exists
    local DASHBOARD_NAME=$1
    local DASHBOARD_SOURCE=$2
    
    exists=$(kubectl get configmap "$DASHBOARD_NAME" >/dev/null 2>&1 ; echo $?)
    if [[ $exists -eq "0" ]]; then
        echo "$DASHBOARD_NAME exists - deleting..."
        kubectl delete configmap "$DASHBOARD_NAME"
    fi
    echo "Creating $DASHBOARD_NAME..."
    kubectl create configmap "$DASHBOARD_NAME" --from-file=./dashboards/"$DASHBOARD_SOURCE"
    #Label it for autodiscovery
    kubectl label configmap "$DASHBOARD_NAME" grafana_dashboard="1"
    echo
}

while [[ $# -gt 1 ]]
do
    case "$2" in 
        *)
            echo "Unexpected '$2'"
            show_usage
            exit 1
            ;;
    esac
done

#
# Main script start
#

# Create/switch to namespace
CURRENT_NAMESPACE=$(kubectl config view --minify --output 'jsonpath={..namespace}')
NAMESPACE="prometheus"
PROMETHEUS_INSTANCE_NAME=${1:-prometheus-operator} # use first argument as instance name, if given
NAMESPACE_EXISTS=$(kubectl get namespaces $NAMESPACE 2>&1 > /dev/null ; echo $?)
if [[ $NAMESPACE_EXISTS = 0 ]]; then
    echo "Namespace $NAMESPACE already exists - skipping"
else
    echo "Creating $NAMESPACE"
    kubectl create namespace $NAMESPACE
fi
echo

echo "Switching to $NAMESPACE namespace"
kubectl config set-context --current --namespace=$NAMESPACE

deploy_dashboard cronprimer-dashboard cronprimer-dash.json

OPERATOR_INSTALLED=$(helm ls -o json | jq '.[] | select(.name=='\"$PROMETHEUS_INSTANCE_NAME\"')  | {name:.name} | length')
if [[ $OPERATOR_INSTALLED -eq "1" ]]; then
    echo "Prometheus operator already installed"
else
    echo "Installing Prometheus operator..."
    # prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false means all serviceMonitors are discover not just 
    # those deployed by the helm chart itself
    helm install $PROMETHEUS_INSTANCE_NAME stable/prometheus-operator --set prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false
fi
echo

echo "Switching to back to orginal namespace: $CURRENT_NAMESPACE"
kubectl config set-context --current --namespace=$CURRENT_NAMESPACE
echo

echo "DONE"
echo "Connect to the grafana:"
echo "kubectl port-forward service/$PROMETHEUS_INSTANCE_NAME-grafana 8080:80 -n $NAMESPACE"
