# Targets

The target section is for target dependent files.

```yaml
targets:
    lxc:
        create_message: <string>
        config:
            - type: <string>
              before: <uint>
              after: <uint>
              content: <string>
            - ...
    lxd:
        vm:
            size: <uint>
            filesystem: <string>
            filesystem_options:
                - options:
                      - <string>
                  releases:
                      - <string>
                  architectures:
                      - <string>
                  variants:
                      - <string>
```

## LXC

The `create_message` field is a string which is displayed after new LXC container has been created.
This string is rendered using Pongo2 and can include various fields from the definition file, e.g. `{{ image.description }}`.

`config` is a list of container configuration options.
The `type` must be `all`, `system` or `user`.

The keys `before` and `after` are used for compatibility.
Currently, the maximum value for compatibility is 5.
If your desired compatibility level is 3 for example, you would use `before: 4` and `after: 2`.

`content` describes the configuration which is to be written to the configuration file.

## LXD

Valid keys are `size`, `filesystem`, and `filesystem_options`.
`size` specifies the VM image size in bytes.
`filesystem` specifies the root partition file system.
It currently supports `ext4` and `btrfs`.
`filesystem_options` is a list of filtered entries whose `options` are passed as additional arguments to `mkfs.<filesystem>` (for example, `["-O", "^metadata_csum_seed,^orphan_file"]` for older releases whose GRUB or `e2fsck` does not support newer ext4 features).
