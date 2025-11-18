# LXD image builder

This repository provides `lxd-imagebuilder` for building system container and virtual machine images
for LXD, and `simplestream-maintainer` for managing images on the simple streams server.

See https://canonical-lxd-imagebuilder.readthedocs-hosted.com/ for documentation.

## Status
Type            | Service               | Status
---             | ---                   | ---
CI              | GitHub                | [![Build Status](https://github.com/canonical/lxd-imagebuilder/workflows/Tests/badge.svg)](https://github.com/canonical/lxd-imagebuilder/actions)



<!-- Include start installing -->
## Installing from package

The `lxd-imagebuilder` and `simplestream-maintainer` tools are available in the `lxd-imagebuilder`
snap from the [Snap Store](https://snapcraft.io/lxd-imagebuilder).

```
sudo snap install lxd-imagebuilder --classic --edge
```

## Installing from source

To compile from source, first install the Go programming language, and some other dependencies.

- Debian-based:
    ```
    sudo apt update
    sudo apt install -y golang-go debootstrap rsync gpg squashfs-tools git make xdelta3
    ```

- ArchLinux-based:
    ```
    sudo pacman -Syu
    sudo pacman -S go debootstrap rsync gnupg squashfs-tools git make xdelta3 --needed
    ```

NOTE: If your package manager doesn't provide a recent enough version, [get it from upstream](https://go.dev/doc/install).

Second, download the source code of the `lxd-imagebuilder` repository (this repository).

```
mkdir -p $HOME/go/src/github.com/canonical/
cd $HOME/go/src/github.com/canonical/
git clone https://github.com/canonical/lxd-imagebuilder
```

Third, enter the directory with the source code of `lxd-imagebuilder` and run `make` to compile the
source code. This will generate the executable programs `lxd-imagebuilder` and `simplestream-maintainer`
in `$HOME/go/bin`.

```
cd ./lxd-imagebuilder
make
```

You may also add the directory `$HOME/go/bin/` to your $PATH so that you do not need to run the command with the full path.
<!-- Include end installing -->

## How to use

See [How to use `lxd-imagebuilder`](doc/howto/build.md) for instructions.

## Troubleshooting

See [Troubleshoot `lxd-imagebuilder`](doc/howto/troubleshoot.md).
