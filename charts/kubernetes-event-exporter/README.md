# kubernetes-event-exporter Helm Chart

## Get Repository Info

```shell
helm repo add kubernetes-event-exporter https://resmoio.github.io/kubernetes-event-exporter
helm update
```

## Prepare the Deployment configuration [Optional]

Create a helm values file.

```yaml
# config.yaml
---
config: |
  logLevel: error
  logFormat: json
  route:
    routes:
      - match:
          - receiver: "dump"
  receivers:
    - name: "dump"
      stdout: {}
```

See [App Configuration](../../README.md) for reference.

## Install Chart

Ensure `monitoring` namespace exists

```shell
kubectl create ns monitoring
```

Install chart

```bash
helm install kubernetes-event-exporter -f config.yaml resmoio/kubernetes-event-exporter
```
