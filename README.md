# Raven-controller-manager

**IMPORTANT: This project is no longer being actively maintained and has been archived.**

## Archived Project

This project has been archived and is no longer being actively maintained. This means you can view and copy the code, but cannot make changes or propose pull requests.

While you're here, feel free to review the code and learn from it. If you wish to use the code or revive the project, you can fork it to your own GitHub account.

## Project Description

Raven-controller-manager is the controller for [Gateway](https://github.com/openyurtio/raven-controller-manager/blob/main/pkg/ravencontroller/apis/raven/v1alpha1/gateway_types.go) CRD.
This project should be used together with the Raven project, which provides network connectivity among pods in different physical regions or network regions.

For a complete example, please check out the [tutorial](https://github.com/openyurtio/raven/blob/main/docs/raven-agent-tutorial.md).

## Previous Contributions

We want to take a moment to thank all of the previous contributors to this project. Your work has been greatly appreciated and has made a significant impact.

- [njucjc](https://github.com/njucjc)
- [DrmagicE](https://github.com/DrmagicE)
- [BSWANG](https://github.com/BSWANG)
- [luckymrwang](https://github.com/luckymrwang)
- [xavier-hou](https://github.com/xavier-hou)
- [rambohe-ch](https://github.com/rambohe-ch)

## Alternative Projects

All the functions of this project have been migrated into `yurt-manager` component in [openyurt](https://github.com/openyurtio/openyurt) repo.

- [controllers](https://github.com/openyurtio/openyurt/tree/master/pkg/yurtmanager/controller/raven)
- [webhooks](https://github.com/openyurtio/openyurt/tree/master/pkg/yurtmanager/webhook/gateway)
