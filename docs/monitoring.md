# Monitoring the Operator

The Operator provides a mechanism for monitoring its performance and throughput via usage of Prometheus. Prometheus is a monitoring and metric gathering tool for Kubernetes and [information regarding the system can be found here](https://github.com/coreos/prometheus-operator). 

This repository provides a way for you to use a new installation of Prometheus as part of the installation of Operator, or to use with an existing installation.
> *Note: In order to scrape the metrics the Operator provides in an existing, it may be necesary to install the custom ServiceMonitor provided within this repo.*

## Installing Prometheus

If you are using a brand new cluster and want to enable monitoring we provide a very simple setup process:

### Prerequisites

- Helm (v3+)
- Terminal with kubeconfig pointing to desired K8s cluster 
- Optional: Helm installs the operator into the currently active context namespace (by default this is `default`). If you wish to install Prometheus into a specific namespace then you should setup your namespace before running the commands below (`kubectl config set-context --current --namespace=<insert-namespace-name-here>`)

### Install Prometheus-Operator Helm chart

1. Deploy the Prometheus-Operator helm chart:
```bash
make install-prometheus
```

2. Ensure Prometheus installs correctly:

```bash
kubectl get pods -l "release=prometheus-operator" -A
```

3. Verify that you see the following output:

```
NAMESPACE   NAME                                                 READY   STATUS    RESTARTS   AGE
default     prometheus-operator-grafana-74df55f54d-znr7k         2/2     Running   0          2m42s
default     prometheus-operator-operator-64f6586685-d54h4        2/2     Running   0          2m42s
default     prometheus-operator-prometheus-node-exporter-x29rf   1/1     Running   0          2m42s
```

> **Notes:** 
> - By default using the `make` command the Helm chart is installed with the name `prometheus-operator`, as seen above in the prefix of the pod names.
> - The name prefix can be overriden when using the `make` command if required: `make install-prometheus {PROMETHEUS_INSTANCE_NAME}=<some value>`. (This is useful if you want to run multiple Prometheus instances per cluster)
> - If you override `{PROMETHEUS_INSTANCE_NAME}` you will need to [make changes to the Kustomization scripts](Customizing%20Installation) and replace the `prometheus-operator` pod label selector in step 2. above.


## Installing the ServiceMonitor

Once your Prometheus instance is up and running correctly you will have to configure the instace to scrape the metrics from the PreScaledCronJob Operator.

The service monitor will automatically get installed during deployment via the use of `make deploy` and `make deploy-cluster` commands providing the `[PROMETHEUS]` sections of the `/config/default/kustomization.yaml` file are uncommented.

## Customizing Installation

These steps need to be performed if you provided a custom `{PROMETHEUS_INSTANCE_NAME}` parameter during installation or if you are using an existing Prometheus installation on a cluster:

1. Determine the `serviceMonitorSelector` being used by your Prometheus instance:

```bash
kubectl get prometheus -o custom-columns="NAME:metadata.name,RELEASE:spec.serviceMonitorSelector"
```

> Example:
> 
> Executing the command gives:
>```
>NAME                                    RELEASE
>prom-prometheus-operator-prometheus1   map[matchLabels:map[release:wibble]]
>prom-prometheus-operator-prometheus2   map[matchLabels:map[release:wobble]]
>```
>
> I want to use Prometheus instance `prom-prometheus-operator-prometheus2` to scrape my metrics so I note the matchLabel is `release:wobble`

2. Edit `config/prometheus/monitor.yaml` file where indicated to match the matchLabel determined in step 1.
3. Install the service monitor via the deployment scripts `make deploy` or `make deploy-cluster`

## Viewing the metrics

To monitor the metrics you will need to port forward to the Prometheus pod from your terminal:

```bash
kubectl port-forward service/prometheus-operator-prometheus 9090:9090 -n <your prometheus namespace>
```

You can now access the metrics on your machine by opening a browser and navigating to `http://localhost:9090`

> **Notes:**
> - If you changed the name of the Prometheus instance then you need to replace the initial `prometheus-operator` above with your instance name *(you can find this by doing `kubectl get services -A` and looking for `prometheus-operator-prometheus`)*
> - If you are using the dev container the port forward may not work, use the [VSCode temporary port forwarding](https://code.visualstudio.com/docs/remote/containers#_temporarily-forwarding-a-port) to resolve

## Viewing Grafana dashboards

The Prometheus-Operator Helm chart comes with an installation of Grafana by default to allow easy installation and viewing of metrics. To view the dashboard you will need to port forward to the service:

```bash
kubectl port-forward service/prometheus-operator-grafana 8080:80 -n <your prometheus namespace>
```

You can now access the metrics on your machine by opening a browser and navigating to `http://localhost:8080`

> **Notes:**
> - Grafana requires a username and password to access. By default the admin password is set via Helm, [this can be found and can be overriden via instructions here](https://github.com/helm/charts/tree/master/stable/prometheus-operator#grafana)
> - If you changed the name of the Prometheus instance then you need to replace `prometheus-operator` above with your instance name *(you can find this by doing `kubectl get services -A` and looking for `-grafana`)*
> - If you are using the dev container the port forward may not work, use the [VSCode temporary port forwarding](https://code.visualstudio.com/docs/remote/containers#_temporarily-forwarding-a-port) to resolve

## Note 

The metrics for `prescalecronjoboperator_cronjob_time_*` published out in the [metrics.go](./controllers/metrics.go) are useful to tell how long the `PrescaledCronJob` took to execute. 

They are stored as a histogram in `prometheus` with exponential buckets starting from 2secs -> 1hr. Once running it's strongly suggested to tweak these buckets based on the observed delays and scale up times.
		
