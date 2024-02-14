# LXD Imagebuilder
System container and VM image builder for LXD and LXC.

## Status
Type            | Service               | Status
---             | ---                   | ---
CI              | GitHub                | [![Build Status](https://github.com/canonical/lxd-imagebuilder/workflows/CI%20tests/badge.svg)](https://github.com/canonical/lxd-imagebuilder/actions)
Project status  | CII Best Practices    | [![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/1728/badge)](https://bestpractices.coreinfrastructure.org/projects/1728)


## Command line options

<!-- Include start CLI -->
The following are the command line options of `lxd-imagebuilder`. You can use `lxd-imagebuilder` to create container images for both LXD and LXC.

```bash
$ lxd-imagebuilder
System container and VM image builder for LXD and LXC

Usage:
  lxd-imagebuilder [command]

Available Commands:
  build-dir      Build plain rootfs
  build-lxc      Build LXC image from scratch
  build-lxd      Build LXD image from scratch
  help           Help about any command
  pack-lxc       Create LXC image from existing rootfs
  pack-lxd       Create LXD image from existing rootfs
  repack-windows Repack Windows ISO with drivers included

Flags:
      --cache-dir         Cache directory
      --cleanup           Clean up cache directory (default true)
      --debug             Enable debug output
      --disable-overlay   Disable the use of filesystem overlays
  -h, --help              help for lxd-imagebuilder
  -o, --options           Override options (list of key=value)
  -t, --timeout           Timeout in seconds
      --version           Print version number

Use "lxd-imagebuilder [command] --help" for more information about a command.

```
<!-- Include end CLI -->

<!-- Include start installing -->
## Installing from package

`lxd-imagebuilder` is available from the [Snap Store](https://snapcraft.io/lxd-imagebuilder).

```
sudo snap install lxd-imagebuilder --classic
```

## Installing from source

To compile `lxd-imagebuilder` from source, first install the Go programming language, and some other dependencies.

- Debian-based:
    ```
    sudo apt update
    sudo apt install -y golang-go debootstrap rsync gpg squashfs-tools git make
    ```

- ArchLinux-based:
    ```
    sudo pacman -Syu
    sudo pacman -S go debootstrap rsync gnupg squashfs-tools git make --needed
    ```

NOTE: Imagebuilder requires Go 1.21 or higher, if your imagebuilder doesn't have a recent enough version available, [get it from upstream](https://go.dev/doc/install).

Second, download the source code of the `lxd-imagebuilder` repository (this repository).

```
mkdir -p $HOME/go/src/github.com/canonical/
cd $HOME/go/src/github.com/canonical/
git clone https://github.com/canonical/lxd-imagebuilder
```

Third, enter the directory with the source code of `lxd-imagebuilder` and run `make` to compile the source code. This will generate the executable program `lxd-imagebuilder`, and it will be located at `$HOME/go/bin/lxd-imagebuilder`.

```
cd ./lxd-imagebuilder
make
```

Finally, you can run `lxd-imagebuilder` as follows.
```
$HOME/go/bin/lxd-imagebuilder
```

You may also add the directory `$HOME/go/bin/` to your $PATH so that you do not need to run the command with the full path.
<!-- Include end installing -->

## How to use

See [How to use `lxd-imagebuilder`](doc/howto/build.md) for instructions.

## Troubleshooting

See [Troubleshoot `lxd-imagebuilder`](doc/howto/troubleshoot.md).
