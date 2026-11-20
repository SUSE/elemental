# Troubleshooting an installed system


## Using `toolbox` for debugging

Operating Systems deployed by Elemental are image based and immutable during
runtime. That solution has several benefits, but it also has some
disadvantages. If you need to debug your system, you need to be able to get
access to additional tools with minimal changes on the host system. For this
reason, a utility called `toolbox` has been developed.

`toolbox` is a small script that pulls a container image and runs a privileged
container based on that image. toolbox is stateful so if you exit the container
and start it later, the environment is exactly the same.

The root file system of the container is mounted on `/media/root`.

## Starting and removing `toolbox`

To start the toolbox container run the following command as root:

```shell
# toolbox
```

If the script completes successfully, you can see the toolbox container prompt.

To remove the container, run the following command:

```shell
# podman rm toolbox-root
```

## Using `toolbox`

In the toolbox container, you can install any tool you want with `zypper` and
then use the tool without rebooting your system. It does not require the system
to be registered, as a subset of SUSE Linux Enterprise Server packages are
available for installation.

To leave the container, just type `exit`. Remember that the container stays in
the same state as you exit it. If you want a clean environment, you need to
remove the toolbox container first.


## Kubernetes resources were not deployed

The Helm charts and Kubernetes manifests of the configuration are deployed on the initializing node by `k8s-resource-installer.service` at first boot. A Helm chart is waited for until the Helm Controller has installed it or given up on it, so a slow chart is not a failed one. A failed resource, e.g. a Helm chart with wrong repository credentials, does not prevent the other resources from being deployed, not even a failing core RKE2 Helm chart. Once all resources were attempted, the service fails and lists the resources that failed to deploy:

```shell
journalctl -u k8s-resource-installer.service
```

The service is restarted up to five times within an hour, which only helps when the cause was fixed in the meantime: existing resources are kept, and the Helm Controller retries a failed chart only once its HelmChart resource changes. To verify a fix on the node, delete the affected HelmChart resources and retry manually:

```shell
kubectl -n kube-system delete helmchart <name>
systemctl reset-failed k8s-resource-installer.service
systemctl start k8s-resource-installer.service
```
