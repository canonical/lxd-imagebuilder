# Command line options

The following are the command line options of `lxd-imagebuilder`.
You can use `lxd-imagebuilder` to create container and VM images for LXD.

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
