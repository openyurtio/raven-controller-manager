# Raven-controller-manager

<div align="left">

[![Version](https://img.shields.io/badge/RavenControllerManager-v0.1.0-orange)](https://github.com/openyurtio/raven-controller-manager/releases/tag/v0.1.0)
[![License](https://img.shields.io/badge/license-Apache%202-4EB1BA.svg)](https://www.apache.org/licenses/LICENSE-2.0.html)
[![Go Report Card](https://goreportcard.com/badge/github.com/openyurtio/raven-controller-manager)](https://goreportcard.com/report/github.com/openyurtio/raven-controller-manager)
[![codecov](https://codecov.io/gh/openyurtio/raven-controller-manager/branch/main/graph/badge.svg)](https://codecov.io/gh/openyurtio/raven-controller-manager)
</div>

Raven-controller-manager is the controller for [Gateway](https://github.com/openyurtio/raven-controller-manager/blob/main/pkg/ravencontroller/apis/raven/v1alpha1/gateway_types.go) CRD.
This project should be used together with the Raven project, which provides network connectivity among pods in different physical regions or network regions.

For a complete example, pleas check out the [tutorial](https://github.com/openyurtio/raven/blob/main/docs/raven-agent-tutorial.md).

## Getting Start

### Build and push raven-controller-manager

```bash
REPO={Your_Docker_Image_Repository} make push
```

The above command will do the following tasks:

* Build an image {Your_Docker_Image_Repository}:{Git_Commit_Id} and push it to your own repository.
* Generate a file named `raven-controller-manager.yaml` in `_output/yamls` dir.

### Install raven-controller-manager

After the raven-controller-manager image is pushed and the `raven-controller-manager.yaml` is generated,
use the following command to install raven-controller-manager into your cluster:

```bash
kubectl apply -f _output/yamls/raven-controller-manager.yaml
```

Then wait for the raven-controller-manager to be created successfully.

```bash
$ kubectl get pod -n kube-system |grep raven-controller-manager
raven-controller-manager-787d69f4bc-l55gp          1/1     Running   1          5m55s
raven-controller-manager-787d69f4bc-tksqq          1/1     Running   0          5m4s
```

## Contributing

Contributions are welcome, whether by creating new issues or pull requests. See
our [contributing document](https://github.com/openyurtio/openyurt/blob/master/CONTRIBUTING.md) to get started.

## Contact

* Mailing List: openyurt@googlegroups.com
* Slack: [channel](https://join.slack.com/t/openyurt/shared_invite/zt-iw2lvjzm-MxLcBHWm01y1t2fiTD15Gw)
* Dingtalk Group (钉钉讨论群)

<div align="left">
    <img src="https://github.com/openyurtio/openyurt/blob/master/docs/img/ding.jpg" width=25% title="dingtalk">
</div>

## License

Raven is under the Apache 2.0 license. See the [LICENSE](LICENSE) file
for details. Certain implementations in Raven rely on the existing code
from [Kubernetes](https://github.com/kubernetes/kubernetes) the credits go to the
original authors.
