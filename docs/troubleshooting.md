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


## Upgrading after an interrupted upgrade

An upgrade creates a new snapshot of the system and only makes it the default once it is complete. A host that loses
power or is reset during an upgrade therefore boots the snapshot it was running before. What the interrupted upgrade
created stays on disk:

* A root snapshot that `snapper list` shows with the user data `update-in-progress=yes`. `snapper delete` refuses to
  remove it ("Directory not empty"), because it contains the nested subvolume of `/etc`.
* A snapshot of `/etc` with the description `pre-transaction /etc snapshot`, listed by `snapper -c etc list`.

Nothing has to be cleaned up by hand. The next `elemental3ctl upgrade` removes the snapshot of the interrupted upgrade
and the stale `/etc` snapshots before it creates its own, and reports this with a warning.
