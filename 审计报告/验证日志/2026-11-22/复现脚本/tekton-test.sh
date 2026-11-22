set -u
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
kubectl create ns tekton-lab >/dev/null 2>&1 || true
cat <<'YAML' | kubectl apply -n tekton-lab -f - 2>&1 | tail -4
apiVersion: tekton.dev/v1
kind: Task
metadata: {name: build-gate}
spec:
  params:
  - {name: coverage, type: string, default: "0.82"}
  steps:
  - name: gate
    image: alpine:3.20
    script: |
      #!/bin/sh
      set -e
      echo "coverage=$(params.coverage)"
      awk -v c="$(params.coverage)" 'BEGIN { if (c+0 < 0.80) { print "GATE FAILED"; exit 1 } print "GATE PASSED" }'
---
apiVersion: tekton.dev/v1
kind: Pipeline
metadata: {name: ci-pipeline}
spec:
  tasks:
  - name: gate
    taskRef: {name: build-gate}
YAML
echo "== Task/Pipeline 已建"
kubectl -n tekton-lab get task,pipeline 2>&1 | tail -4
echo "== 直接跑 TaskRun（应 Succeeded）"
cat <<'YAML' | kubectl apply -n tekton-lab -f - >/dev/null 2>&1
apiVersion: tekton.dev/v1
kind: TaskRun
metadata: {name: gate-ok, generateName: gate-ok-}
spec:
  taskRef: {name: build-gate}
  params: [{name: coverage, value: "0.86"}]
YAML
kubectl -n tekton-lab wait --for=condition=Succeeded taskrun/gate-ok --timeout=300s 2>&1 | tail -2
kubectl -n tekton-lab get taskrun gate-ok -o jsonpath='{.status.conditions[0].reason}{" "}{.status.conditions[0].status}{"\n"}'
echo "== 跑一个会失败的 TaskRun（应 Failed，验证门禁真能拦）"
cat <<'YAML' | kubectl apply -n tekton-lab -f - >/dev/null 2>&1
apiVersion: tekton.dev/v1
kind: TaskRun
metadata: {name: gate-bad}
spec:
  taskRef: {name: build-gate}
  params: [{name: coverage, value: "0.61"}]
YAML
sleep 20
kubectl -n tekton-lab get taskrun gate-bad -o jsonpath='{.status.conditions[0].reason}{" "}{.status.conditions[0].status}{"\n"}' 2>/dev/null
echo "== 跑 PipelineRun"
cat <<'YAML' | kubectl apply -n tekton-lab -f - >/dev/null 2>&1
apiVersion: tekton.dev/v1
kind: PipelineRun
metadata: {name: ci-run}
spec:
  pipelineRef: {name: ci-pipeline}
YAML
kubectl -n tekton-lab wait --for=condition=Succeeded pipelinerun/ci-run --timeout=300s 2>&1 | tail -2
kubectl -n tekton-lab get pipelinerun ci-run -o jsonpath='{.status.conditions[0].reason}{" "}{.status.conditions[0].status}{"\n"}'
echo "== gate 日志"; kubectl -n tekton-lab logs -l tekton.dev/task=build-gate --tail=20 2>/dev/null | grep -E "coverage|GATE" | tail -4
echo "== 版本"; kubectl -n tekton-pipelines get deploy tekton-pipelines-controller -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
