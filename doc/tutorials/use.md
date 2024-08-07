# Use `lxd-imagebuilder` to create images

This guide shows you how to create an image for LXD or LXC.

Before you start, you must install `lxd-imagebuilder`.
See [How to install `lxd-imagebuilder` and `simplestream-maintainer`](/howto/install) for instructions.

## Create an image

To create an image, first create a directory where you will be placing the images, and enter that directory.

```
mkdir -p $HOME/Images/ubuntu/
cd $HOME/Images/ubuntu/
```

Then, copy one of the example YAML configuration files for images into this directory.

```{note}
The YAML configuration file contains an image template that gives instructions to LXD imagebuilder.

LXD imagebuilder provides examples of YAML files for various distributions in the [examples directory](https://github.com/canonical/lxd-imagebuilder/tree/main/doc/examples).
[`scheme.yaml`](https://github.com/canonical/lxd-imagebuilder/blob/main/doc/examples/scheme.yaml) is a standard template that includes all available options.

Official LXD templates for various distributions are available in the [`lxd-ci` repository](https://github.com/canonical/lxd-ci/tree/main/images).
```

In this example, we are creating an Ubuntu image.

```
cp $HOME/go/src/github.com/canonical/lxd-imagebuilder/doc/examples/ubuntu.yaml ubuntu.yaml
```

### Edit the template file

Optionally, you can do some edits to the YAML configuration file.
You can define the following keys:

| Section    | Description                                                                              | Documentation                                   |
|------------|------------------------------------------------------------------------------------------|-------------------------------------------------|
| `image`    | Defines distribution, architecture, release etc.                                         | {doc}`../reference/lxd-imagebuilder/image`      |
| `source`   | Defines main package source, keys etc.                                                   | {doc}`../reference/lxd-imagebuilder/source`     |
| `targets`  | Defines configuration for specific targets (e.g. LXD, instances etc.)                    | {doc}`../reference/lxd-imagebuilder/targets`    |
| `files`    | Defines generators to modify files                                                       | {doc}`../reference/lxd-imagebuilder/generators` |
| `packages` | Defines packages for install or removal; adds repositories                               | {doc}`../reference/lxd-imagebuilder/packages`   |
| `actions`  | Defines scripts to be run after specific steps during image building                     | {doc}`../reference/lxd-imagebuilder/actions`    |
| `mappings` | Maps different terms for architectures for specific distributions (e.g. `x86_64: amd64`) | {doc}`../reference/lxd-imagebuilder/mappings`   |

```{tip}
When building a VM image, you should either build an image with cloud-init support (provides automatic size growth) or set a higher size in the template, because the standard size is relatively small (10 GiB). Alternatively, you can also grow it manually.
```

## Build and launch the image

The steps for building and launching the image depend on whether you want to use it with LXD or with LXC.

### Create an image for LXD

To build an image for LXD, run `lxd-imagebuilder`. We are using the `build-lxd` option to create an image for LXD.

- To create a container image:

 ```bash
 sudo $HOME/go/bin/lxd-imagebuilder build-lxd ubuntu.yaml
 ```

- To create a VM image:

 ```bash
 sudo $HOME/go/bin/lxd-imagebuilder build-lxd ubuntu.yaml --vm
 ```

See {ref}`howto-build-lxd` for more information about the `build-lxd` command.

If the command is successful, you will get an output similar to the following (for a container image). The `lxd.tar.xz` file is the description of the container image. The `rootfs.squasfs` file is the root file system (rootfs) of the container image. The set of these two files is the _container image_.

```bash
$ ls -l
total 100960
-rw-r--r-- 1 root   root         676 Oct  3 16:15 lxd.tar.xz
-rw-r--r-- 1 root   root   103370752 Oct  3 16:15 rootfs.squashfs
-rw-r--r-- 1 ubuntu ubuntu      7449 Oct  3 16:03 ubuntu.yaml
$
```

#### Add the image to LXD

To add the image to an LXD installation, use the `lxc image import` command as follows.

```bash
$ lxc image import lxd.tar.xz rootfs.squashfs --alias mycontainerimage
Image imported with fingerprint: 009349195858651a0f883de804e64eb82e0ac8c0bc51880
```

See {ref}`lxd:images-copy` for detailed information.

Let's look at the image in LXD. The `ubuntu.yaml` had a setting to create an Ubuntu 20.04 (`focal`) image. The size is 98.58MB.

```bash
$ lxc image list mycontainerimage
+------------------+--------------+--------+--------------+--------+---------+-----------------------------+
|      ALIAS       | FINGERPRINT  | PUBLIC | DESCRIPTION  |  ARCH  |  SIZE   |         UPLOAD DATE         |
+------------------+--------------+--------+--------------+--------+---------+-----------------------------+
| mycontainerimage | 009349195858 | no     | Ubuntu focal | x86_64 | 98.58MB | Oct 3, 2020 at 5:10pm (UTC) |
+------------------+--------------+--------+--------------+--------+---------+-----------------------------+
```

#### Launch an LXD container from the container image

To launch a container from the freshly created image, use `lxc launch` as follows. Note that you do not specify a repository for the image (like `ubuntu:` or `images:`) because the image is located locally.

```bash
$ lxc launch mycontainerimage c1
Creating c1
Starting c1
```

### Create an image for LXC

Using LXC containers instead of LXD may require the installation of `lxc-utils`.
Having both LXC and LXD installed on the same system will probably cause confusion.
Use of raw LXC is generally discouraged due to the lack of automatic AppArmor
protection.

To create an image for LXC, use the following command:

```bash
$ sudo $HOME/go/bin/lxd-imagebuilder build-lxc ubuntu.yaml
$ ls -l
total 87340
-rw-r--r-- 1 root root      740 Jan 19 03:15 meta.tar.xz
-rw-r--r-- 1 root root 89421136 Jan 19 03:15 rootfs.tar.xz
-rw-r--r-- 1 root root     4798 Jan 19 02:42 ubuntu.yaml
```

See {ref}`howto-build-lxc` for more information about the `build-lxc` command.

#### Add the container image to LXC

To add the container image to a LXC installation, use the `lxc-create` command as follows.

```bash
lxc-create -n myContainerImage -t local -- --metadata meta.tar.xz --fstree rootfs.tar.xz
```

#### Launch a LXC container from the container image

Then start the container with

```bash
lxc-start -n myContainerImage
```

## Repack Windows ISO

```{youtube} https://www.youtube.com/watch?v=3PDMGwbbk48
```

With LXD it's possible to run Windows VMs. All you need is a Windows ISO and a bunch of drivers.
To make the installation a bit easier, `lxd-imagebuilder` added the `repack-windows` command. It takes a Windows ISO, and repacks it together with the necessary drivers.

The `lxd-imagebuilder` will automatically detect the Windows version, however, it is possible to set the version manually using `--windows-version` flag, which allows the following values:

Flag value | Version
---------- | ----------------------
`w11`      | Windows 11
`w10`      | Windows 10
`w8`       | Windows 8
`w8.1`     | Windows 8.1
`w7`       | Windows 7
`xp`       | Windows XP
`2k22`     | Windows Server 2022
`2k19`     | Windows Server 2019
`2k16`     | Windows Server 2016
`2k12`     | Windows Server 2012
`2k12r2`   | Windows Server 2012 R2
`2k8`      | Windows Server 2008
`2k8r2`    | Windows Server 2008 R2
`2k3`      | Windows Server 2003

When repacking a Windows ISO, `lxd-imagebuilder` uses external tools that may need to be installed. On a Ubuntu/Debian system, those can be installed with:

```bash
sudo apt-get install -y --no-install-recommends genisoimage libwin-hivex-perl rsync wimtools
```

Here's how to repack a Windows ISO:

```bash
sudo lxd-imagebuilder repack-windows path/to/Windows.iso path/to/Windows-repacked.iso
```

More information on `repack-windows` can be found by running

```bash
lxd-imagebuilder repack-windows -h
```

### Install Windows

Run the following commands to initialize the VM, add a TPM device, increase the allocated disk space, CPU, memory and finally attach the full path of your prepared ISO file.

```bash
lxc init win11 --empty --vm
lxc config device add win11 vtpm tpm path=/dev/tpm0
lxc config device override win11 root size=50GiB
lxc config set win11 limits.cpu=4 limits.memory=8GiB
lxc config device add win11 iso disk source=/path/to/Windows-repacked.iso boot.priority=10
```

Now, the VM win11 has been configured and it is ready to be started. The following command starts the virtual machine and opens up a VGA console so that we go through the graphical installation of Windows.

```bash
lxc start win11 --console=vga
```

Once done with the manual installation process, the ISO can be removed to speed up next boots by avoiding the prompt to boot from it.

```bash
lxc config device remove win11 iso
```
