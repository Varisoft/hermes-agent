# Hermes Kubernetes Operator

MVP Kubernetes operator for running Hermes Agent instances from a `HermesAgent`
custom resource.

## Build and Test

```bash
make test
```

`make test` runs controller-gen, gofmt, go vet, and unit tests.

## Install CRD

```bash
make install
```

## Deploy Controller

```bash
make deploy IMG=ghcr.io/your-org/hermes-operator:latest
```

## Create an Instance

Create a Kubernetes Secret containing Hermes environment values first:

```bash
kubectl create secret generic coder-hermes-secrets \
  --from-literal=OPENAI_API_KEY=... \
  --from-literal=API_SERVER_KEY=change-me
```

Then apply a `HermesAgent`:

```bash
kubectl apply -f config/samples/hermes_v1alpha1_hermesagent.yaml
```

Each instance reconciles to one PVC mounted as `/opt/data`, one ConfigMap used
to seed `config.yaml`/`SOUL.md`, one StatefulSet running `gateway run`, and a
ClusterIP Service only when the dashboard or API server is enabled. PVCs are
retained on CR deletion by default.
