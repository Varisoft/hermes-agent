# Hermes Kubernetes Operator

Hermes Kubernetes Operator เป็น MVP operator สำหรับรัน `Hermes Agent`
บน Kubernetes ผ่าน custom resource ชนิด `HermesAgent`

แทนที่จะต้องเขียน `StatefulSet`, `Service`, `PersistentVolumeClaim`,
และ `ConfigMap` เองทั้งหมด คุณสามารถประกาศแค่ `HermesAgent` แล้วปล่อยให้
operator reconcile resource ที่เกี่ยวข้องให้เอง

## Operator ตัวนี้สร้างอะไรให้บ้าง

เมื่อสร้าง `HermesAgent` 1 ตัว operator จะสร้าง resource หลักให้ดังนี้

- `PersistentVolumeClaim` สำหรับเก็บ `HERMES_HOME` ที่ `/opt/data`
- `ConfigMap` สำหรับ seed `config.yaml` และ `SOUL.md`
- `StatefulSet` สำหรับรัน `hermes gateway run`
- `Service` แบบ `ClusterIP` ถ้ามีการเปิด dashboard หรือ API server

ค่า default สำคัญ:

- ถ้าไม่ระบุ `spec.image` จะใช้ `nousresearch/hermes-agent:0.12.0`
- PVC จะถูกเก็บไว้ตอนลบ CR ถ้า `retainOnDelete` ยังเป็นค่า default (`true`)

## สิ่งที่ควรมีก่อนใช้งาน

- Kubernetes cluster ที่ใช้งานได้
- `kubectl` ที่ชี้ไปยัง cluster ถูกตัว
- สิทธิ์สำหรับติดตั้ง CRD และ deploy controller
- image ของ operator ที่พร้อมให้ cluster pull ได้

## Build และ Test

จากโฟลเดอร์ `operators/hermes-operator`

```bash
make test
```

คำสั่งนี้จะรัน `controller-gen`, `gofmt`, `go vet` และ unit tests

## ติดตั้ง CRD

```bash
make install
```

คำสั่งนี้จะติดตั้ง `HermesAgent` CRD ลงใน cluster

## Deploy ตัว Controller

```bash
make deploy IMG=ghcr.io/your-org/hermes-operator:latest
```

เปลี่ยน `IMG` ให้เป็น image ของ operator ที่ cluster ของคุณ pull ได้จริง

## วิธีใช้งาน

### 1. สร้าง Secret สำหรับ Hermes

ตัวอย่างขั้นต่ำ:

```bash
kubectl create secret generic coder-hermes-secrets \
  --from-literal=OPENAI_API_KEY=... \
  --from-literal=API_SERVER_KEY=change-me
```

`OPENAI_API_KEY` ใช้สำหรับให้ Hermes คุยกับ model provider

`API_SERVER_KEY` จำเป็นเมื่อเปิด `spec.apiServer.enabled: true`

### 2. สร้าง HermesAgent

ใช้ sample manifest ที่มีอยู่:

```bash
kubectl apply -f config/samples/hermes_v1alpha1_hermesagent.yaml
```

sample นี้จงใจไม่ระบุ `spec.image` เพื่อให้ operator ใส่ default ให้เอง

### 3. ตรวจสถานะ

```bash
kubectl get hermesagents
kubectl get hermesagent coder -o yaml
kubectl get pvc,sts,svc,pod
```

ถ้า reconcile สำเร็จ จะเห็น resource ที่เกี่ยวข้องถูกสร้างขึ้นและ
`.status.phase` จะเปลี่ยนเป็น `Ready`

### 4. เปิด proxy เพื่อเข้า dashboard

ถ้าเปิด `dashboard.enabled: true` ไว้:

```bash
kubectl port-forward svc/coder 9119:9119
```

จากนั้นเข้าใช้งานที่:

- `http://127.0.0.1:9119/`
- `http://127.0.0.1:9119/chat`

### 5. ลบ resource ตอนเลิกใช้งาน

ลบตัว `HermesAgent`:

```bash
kubectl delete hermesagent coder
```

ถ้าต้องการลบ PVC ด้วย ให้ลบเพิ่มเอง:

```bash
kubectl delete pvc coder-data
```

เหตุผลคือ operator ตั้งค่า default ให้ `retainOnDelete: true`

## ตัวอย่าง manifest

ไฟล์ sample อยู่ที่
`config/samples/hermes_v1alpha1_hermesagent.yaml`

ตัวอย่างสำคัญของ field ใน `spec`

- `image` image ของ Hermes ที่ต้องการใช้ ถ้าไม่ระบุจะใช้ default
- `imagePullPolicy` ควบคุมการ pull image
- `persistence.size` ขนาดของ PVC
- `persistence.retainOnDelete` ลบ CR แล้วจะเก็บ PVC ไว้หรือไม่
- `config` เนื้อหาเริ่มต้นของ `config.yaml`
- `soul` เนื้อหาเริ่มต้นของ `SOUL.md`
- `envFromSecrets` รายชื่อ Kubernetes Secret ที่ต้อง map เป็น env vars
- `dashboard.enabled` เปิด dashboard
- `dashboard.tui` เปิดหน้า chat แบบ embedded TUI
- `apiServer.enabled` เปิด OpenAI-compatible API server
- `apiServer.keySecretRef` secret สำหรับ `API_SERVER_KEY`
- `resources` CPU และ memory requests/limits

## ตัวอย่าง manifest แบบกำหนด image เอง

```yaml
apiVersion: hermes.nousresearch.com/v1alpha1
kind: HermesAgent
metadata:
  name: coder
spec:
  image: nousresearch/hermes-agent:0.12.0
  imagePullPolicy: IfNotPresent
  envFromSecrets:
  - name: coder-hermes-secrets
  dashboard:
    enabled: true
    tui: true
    port: 9119
  apiServer:
    enabled: true
    port: 8642
    keySecretRef:
      name: coder-hermes-secrets
      key: API_SERVER_KEY
```

## หมายเหตุสำคัญ

- operator ตัวนี้ยังเป็น MVP ไม่ได้จัดการ ingress, TLS, autoscaling หรือ public exposure ให้อัตโนมัติ
- ถ้าจะใช้ `latest` ควรพิจารณา `imagePullPolicy` ให้เหมาะสม
- ถ้าต้องการควบคุม version ของ Hermes ให้แน่นอน ควร pin image tag หรือ digest เอง
