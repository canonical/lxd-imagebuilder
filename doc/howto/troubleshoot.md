# How to troubleshoot `lxd-imagebuilder`

This section covers some of the most commonly encountered problems and gives instructions for resolving them.

## Cannot install into target

> Error `Cannot install into target '/var/cache/lxd-imagebuilder.123456789/rootfs' mounted with noexec or nodev`

You have installed `lxd-imagebuilder` into an LXD container and you are trying to run it. `lxd-imagebuilder` does not run in an LXD container. Run `lxd-imagebuilder` on the host, or in a VM.

## Classic confinement

> Error `error: This revision of snap "lxd-imagebuilder" was published using classic confinement`

You are trying to install the `lxd-imagebuilder` snap package. The `lxd-imagebuilder` snap package has been configured to use the `classic` confinement. Therefore, when you install it, you have to add the flag `--classic` as shown above in the instructions.

## Must be root

> Error `You must be root to run this tool`

You must be _root_ in order to run the `lxd-imagebuilder` tool. The tool runs commands such as `mknod` that require administrative privileges. Use `sudo` when running `lxd-imagebuilder`.
