# k8s 自定义调度器逻辑之 Scheduler Framework

通过 Scheduler Framework 创建自己的调度器，新增调度插件自定义调度逻辑。


Scheduler 系列完整内容见：[K8s 自定义调度器 Part2：通过 Scheduler Framework 实现自定义调度逻辑](https://www.lixueduan.com/posts/kubernetes/34-custom-scheduker-by-scheduler-framework/)


## 微信公众号：探索云原生

一个云原生打工人的探索之路，专注云原生，Go，坚持分享最佳实践、经验干货。

扫描下面二维码，关注我即时获取更新~

![](https://img.lixueduan.com/about/wechat/qrcode_search.png)

## 部署

### 构建镜像

将自定义调度器打包成镜像

```bash
REGISTRY=docker.io/lixd96 RELEASE_VERSION=pripority PLATFORMS="linux/amd64,linux/arm64" DISTROLESS_BASE_IMAGE=busybox:1.36 make push-images
```



### 部署到集群

scheduler-plugins 项目下 manifest/install 目录中有部署的 chart，我们只需要修改下 image 参数就行了。

安装命令如下：

```bash
cd manifest/install 

helm upgrade --install i-scheduler charts/as-a-second-scheduler --namespace kube-system -f charts/as-a-second-scheduler/values-demo.yaml
```



完整 values-demo.yaml 内容如下：

```yaml
# Default values for scheduler-plugins-as-a-second-scheduler.
# This is a YAML-formatted file.
# Declare variables to be passed into your templates.

scheduler:
  name: i-scheduler
  image: lixd96/kube-scheduler:pripority

controller:
  replicaCount: 0

# LoadVariationRiskBalancing and TargetLoadPacking are not enabled by default
# as they need extra RBAC privileges on metrics.k8s.io.

plugins:
  enabled: ["Priority","Coscheduling","CapacityScheduling","NodeResourceTopologyMatch","NodeResourcesAllocatable"]
```



deploy.yaml 完整内容如下：

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: i-scheduler
  namespace: kube-system
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: i-scheduler-clusterrolebinding
  namespace: kube-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
  - kind: ServiceAccount
    name: i-scheduler
    namespace: kube-system
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: scheduler-config
  namespace: kube-system
data:
  scheduler-config.yaml: |
    apiVersion: kubescheduler.config.k8s.io/v1
    kind: KubeSchedulerConfiguration
    leaderElection:
      leaderElect: false
    profiles:
    - schedulerName: i-scheduler
      plugins:
        filter:
          enabled:
          - name: Priority
        score:
          enabled:
            - name: Priority
          disabled:
            - name: "*"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: i-scheduler
  namespace: kube-system
spec:
  replicas: 1
  selector:
    matchLabels:
      component: i-scheduler
  template:
    metadata:
      labels:
        component: i-scheduler
    spec:
      serviceAccount: i-scheduler
      priorityClassName: system-cluster-critical
      volumes:
        - name: scheduler-config
          configMap:
            name: scheduler-config
      containers:
        - name: i-scheduler
          image: lixd96/kube-scheduler:pripority
          args:
            - --config=/etc/kubernetes/scheduler-config.yaml
            - --v=3
          volumeMounts:
            - name: scheduler-config
              mountPath: /etc/kubernetes
```

部署到集群

```bash
kubectl apply -f deploy.yaml 
```



确认已经跑起来了

```bash
[root@scheduler-1 install]# kubectl -n kube-system get po|grep i-scheduler
i-scheduler-569bbd89bc-7p5v2                  1/1     Running            0          3m44s
```



## 测试


### 创建 Pod

创建一个 Deployment 并指定使用上一步中部署的 Scheduler，然后测试会调度到哪个节点上。

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test
spec:
  replicas: 1
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      schedulerName: i-scheduler
      containers:
      - image: busybox:1.36
        name: nginx
        command: ["sleep"]         
        args: ["99999"]
```

创建之后 Pod 会一直处于 Pending 状态

```bash
[root@scheduler-1 install]# k get po
NAME                    READY   STATUS    RESTARTS   AGE
test-7f7bb8f449-w6wvv   0/1     Pending   0          4s
```

查看具体情况

```bash
[root@scheduler-1 install]# kubectl describe po test-7f7bb8f449-w6wvv

Events:
  Type     Reason            Age   From         Message
  ----     ------            ----  ----         -------
  Warning  FailedScheduling  8s    i-scheduler  0/2 nodes are available: 1 Node:Node: scheduler-1 does not have label priority.lixueduan.com, 1 Node:Node: scheduler-2 does not have label priority.lixueduan.com. preemption: 0/2 nodes are available: 2 No preemption victims found for incoming pod.
```

可以看到，是因为 Node 上没有我们定义的 Label，因此都不满足条件，最终 Pod 就一直 Pending 了。



### 添加 Label

由于我们实现的 Filter 逻辑是需要 Node 上有`priority.lixueduan.com` 才会用来调度，否则直接会忽略。



理论上，只要给任意一个 Node 打上 Label 就可以了。

```bash
[root@scheduler-1 install]# k get node
NAME          STATUS   ROLES           AGE     VERSION
scheduler-1   Ready    control-plane   4h34m   v1.27.4
scheduler-2   Ready    <none>          4h33m   v1.27.4
[root@scheduler-1 install]# k label node scheduler-1 priority.lixueduan.com=10
node/scheduler-1 labeled
```

再次查看 Pod 状态

```bash
[root@scheduler-1 install]# k get po -owide
NAME                    READY   STATUS    RESTARTS   AGE     IP               NODE          NOMINATED NODE   READINESS GATES
test-7f7bb8f449-w6wvv   1/1     Running   0          4m20s   172.25.123.199   scheduler-1   <none>           <none>
```

已经被调度到 node1 上了，查看详细日志

```bash
[root@scheduler-1 install]# k describe po test-7f7bb8f449-w6wvv
Events:
  Type     Reason            Age   From         Message
  ----     ------            ----  ----         -------
  Warning  FailedScheduling  4m8s  i-scheduler  0/2 nodes are available: 1 Node:Node: scheduler-1 does not have label priority.lixueduan.com, 1 Node:Node: scheduler-2 does not have label priority.lixueduan.com. preemption: 0/2 nodes are available: 2 No preemption victims found for incoming pod.
  Normal   Scheduled         33s   i-scheduler  Successfully assigned default/test-7f7bb8f449-w6wvv to scheduler-1
```

可以看到，也是 i-scheduler 在处理，调度到了 node1.



### 多节点排序

我们实现的 Score 是根据 Node 上的 `priority.lixueduan.com` 对应的 Value 作为得分的，因此肯定会调度到 Value 比较大的一个节点。



给 node2 也打上 label，value 设置为 20

```bash
[root@scheduler-1 install]# k get node
NAME          STATUS   ROLES           AGE     VERSION
scheduler-1   Ready    control-plane   4h34m   v1.27.4
scheduler-2   Ready    <none>          4h33m   v1.27.4
[root@scheduler-1 install]# k label node scheduler-2 priority.lixueduan.com=20
node/scheduler-2 labeled
```

然后更新 Deployment ，触发创建新 Pod ，测试调度逻辑。

因为 Node2 上的 priority 为 20，node1 上为 10，那么肯定会调度到 node2 上。

```bash
[root@scheduler-1 install]# k get po -owide
NAME                    READY   STATUS    RESTARTS   AGE   IP             NODE          NOMINATED NODE   READINESS GATES
test-7f7bb8f449-krvqj   1/1     Running   0          58s   172.25.0.150   scheduler-2   <none>           <none>
```

果然，被调度到了 Node2。



现在我们更新 Node1 的 label，改成 30

```bash
k label node scheduler-1 priority.lixueduan.com=30 --overwrite
```

再次更新 Deployment 触发调度

```bash
[root@scheduler-1 install]# k rollout restart deploy test
deployment.apps/test restarted
```

这样应该是调度到 node1 了，确认一下

```bash
[root@scheduler-1 install]# k get po -owide
NAME                   READY   STATUS    RESTARTS   AGE   IP               NODE          NOMINATED NODE   READINESS GATES
test-f7b597544-bbcb8   1/1     Running   0          65s   172.25.123.200   scheduler-1   <none>           <none>
```

果然在 node1，说明我们的 Scheduler 是能够正常工作的。

---


[![Go Report Card](https://goreportcard.com/badge/kubernetes-sigs/scheduler-plugins)](https://goreportcard.com/report/kubernetes-sigs/scheduler-plugins) [![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://github.com/kubernetes-sigs/scheduler-plugins/blob/master/LICENSE)

# Scheduler Plugins

Repository for out-of-tree scheduler plugins based on the [scheduler framework](https://kubernetes.io/docs/concepts/scheduling-eviction/scheduling-framework/).

This repo provides scheduler plugins that are exercised in large companies.
These plugins can be vendored as Golang SDK libraries or used out-of-box via the pre-built images or Helm charts.
Additionally, this repo incorporates best practices and utilities to compose a high-quality scheduler plugin.

## Install

Container images are available in the official scheduler-plugins k8s container registry. There are two images one
for the kube-scheduler and one for the controller. See the [Compatibility Matrix section](#compatibility-matrix)
for the complete list of images.

```shell
docker pull registry.k8s.io/scheduler-plugins/kube-scheduler:$TAG
docker pull registry.k8s.io/scheduler-plugins/controller:$TAG
```

You can find [how to install release image](doc/install.md) here.

## Plugins

The kube-scheduler binary includes the below list of plugins. They can be configured by creating one or more
[scheduler profiles](https://kubernetes.io/docs/reference/scheduling/config/#multiple-profiles).

* [Capacity Scheduling](pkg/capacityscheduling/README.md)
* [Coscheduling](pkg/coscheduling/README.md)
* [Node Resources](pkg/noderesources/README.md)
* [Node Resource Topology](pkg/noderesourcetopology/README.md)
* [Preemption Toleration](pkg/preemptiontoleration/README.md)
* [Trimaran](pkg/trimaran/README.md)
* [Network-Aware Scheduling](pkg/networkaware/README.md)

Additionally, the kube-scheduler binary includes the below list of sample plugins. These plugins are not intended for use in production
environments.

* [Cross Node Preemption](pkg/crossnodepreemption/README.md)
* [Pod State](pkg/podstate/README.md)
* [Quality of Service](pkg/qos/README.md)

## Compatibility Matrix

The below compatibility matrix shows the k8s client package (client-go, apimachinery, etc) versions
that the scheduler-plugins are compiled with.

The minor version of the scheduler-plugins matches the minor version of the k8s client packages that
it is compiled with. For example scheduler-plugins `v0.18.x` releases are built with k8s `v1.18.x`
dependencies.

The scheduler-plugins patch versions come in two different varieties (single digit or three digits).
The single digit patch versions (e.g., `v0.18.9`) exactly align with the k8s client package
versions that the scheduler plugins are built with. The three digit patch versions, which are built
on demand, (e.g., `v0.18.800`) are used to indicated that the k8s client package versions have not
changed since the previous release, and that only scheduler plugins code (features or bug fixes) was
changed.

| Scheduler Plugins | Compiled With k8s Version | Container Image                                           | Arch                                                       |
|-------------------|---------------------------|-----------------------------------------------------------|------------------------------------------------------------|
| v0.29.7           | v1.29.7                   | registry.k8s.io/scheduler-plugins/kube-scheduler:v0.29.7  | linux/amd64<br>linux/arm64<br>linux/s390x<br>linux/ppc64le |
| v0.28.9           | v1.28.9                   | registry.k8s.io/scheduler-plugins/kube-scheduler:v0.28.9  | linux/amd64<br>linux/arm64                                 |
| v0.27.8           | v1.27.8                   | registry.k8s.io/scheduler-plugins/kube-scheduler:v0.27.8  | linux/amd64<br>linux/arm64                                 |

| Controller | Compiled With k8s Version | Container Image                                       | Arch                                                       |
|------------|---------------------------|-------------------------------------------------------|------------------------------------------------------------|
| v0.29.7    | v1.29.7                   | registry.k8s.io/scheduler-plugins/controller:v0.29.7  | linux/amd64<br>linux/arm64<br>linux/s390x<br>linux/ppc64le |
| v0.28.9    | v1.28.9                   | registry.k8s.io/scheduler-plugins/controller:v0.28.9  | linux/amd64<br>linux/arm64                                 |
| v0.27.8    | v1.27.8                   | registry.k8s.io/scheduler-plugins/controller:v0.27.8  | linux/amd64<br>linux/arm64                                 |

<details>
<summary>Older releases</summary>

| Scheduler Plugins | Compiled With k8s Version | Container Image                                           | Arch                       |
|-------------------|---------------------------|-----------------------------------------------------------|----------------------------|
| v0.26.7           | v1.26.7                   | registry.k8s.io/scheduler-plugins/kube-scheduler:v0.26.7  | linux/amd64<br>linux/arm64 |
| v0.25.12          | v1.25.12                  | registry.k8s.io/scheduler-plugins/kube-scheduler:v0.25.12 | linux/amd64<br>linux/arm64 |
| v0.24.9           | v1.24.9                   | registry.k8s.io/scheduler-plugins/kube-scheduler:v0.24.9  | linux/amd64<br>linux/arm64 |
| v0.23.10          | v1.23.10                  | registry.k8s.io/scheduler-plugins/kube-scheduler:v0.23.10 | linux/amd64<br>linux/arm64 |
| v0.22.6           | v1.22.6                   | registry.k8s.io/scheduler-plugins/kube-scheduler:v0.22.6  | linux/amd64<br>linux/arm64 |
| v0.21.6           | v1.21.6                   | registry.k8s.io/scheduler-plugins/kube-scheduler:v0.21.6  | linux/amd64<br>linux/arm64 |
| v0.20.10          | v1.20.10                  | registry.k8s.io/scheduler-plugins/kube-scheduler:v0.20.10 | linux/amd64<br>linux/arm64 |
| v0.19.9           | v1.19.9                   | registry.k8s.io/scheduler-plugins/kube-scheduler:v0.19.9  | linux/amd64<br>linux/arm64 |
| v0.19.8           | v1.19.8                   | registry.k8s.io/scheduler-plugins/kube-scheduler:v0.19.8  | linux/amd64<br>linux/arm64 |
| v0.18.9           | v1.18.9                   | registry.k8s.io/scheduler-plugins/kube-scheduler:v0.18.9  | linux/amd64                |

| Controller | Compiled With k8s Version | Container Image                                       | Arch                       |
|------------|---------------------------|-------------------------------------------------------|----------------------------|
| v0.26.7    | v1.26.7                   | registry.k8s.io/scheduler-plugins/controller:v0.26.7  | linux/amd64<br>linux/arm64 |
| v0.25.12   | v1.25.12                  | registry.k8s.io/scheduler-plugins/controller:v0.25.12 | linux/amd64<br>linux/arm64 |
| v0.24.9    | v1.24.9                   | registry.k8s.io/scheduler-plugins/controller:v0.24.9  | linux/amd64<br>linux/arm64 |
| v0.23.10   | v1.23.10                  | registry.k8s.io/scheduler-plugins/controller:v0.23.10 | linux/amd64<br>linux/arm64 |
| v0.22.6    | v1.22.6                   | registry.k8s.io/scheduler-plugins/controller:v0.22.6  | linux/amd64<br>linux/arm64 |
| v0.21.6    | v1.21.6                   | registry.k8s.io/scheduler-plugins/controller:v0.21.6  | linux/amd64<br>linux/arm64 |
| v0.20.10   | v1.20.10                  | registry.k8s.io/scheduler-plugins/controller:v0.20.10 | linux/amd64<br>linux/arm64 |
| v0.19.9    | v1.19.9                   | registry.k8s.io/scheduler-plugins/controller:v0.19.9  | linux/amd64<br>linux/arm64 |
| v0.19.8    | v1.19.8                   | registry.k8s.io/scheduler-plugins/controller:v0.19.8  | linux/amd64<br>linux/arm64 |

</details>

## Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

You can reach the maintainers of this project at:

- [Slack](https://kubernetes.slack.com/messages/sig-scheduling)
- [Mailing List](https://groups.google.com/forum/#!forum/kubernetes-sig-scheduling)

You can find an [instruction how to build and run out-of-tree plugin here](doc/develop.md) .

### Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).
