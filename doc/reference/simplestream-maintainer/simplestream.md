# Simple streams configuration

A product version can contain an optional `images.yaml` configuration file.
This file can contain additional image information, such as release aliases or image requirements
which cannot be parsed purely from the directory structure.

All simple streams related configuration is located within a `simplestream` field, which currently
supports:

- `distro_name` - Name of the distribution that is shown when listing images in LXD.
  It defaults to the distribution name parsed from the directory structure.
- `release_aliases` - A map of the distribution release and a comma-delimited string of release
  aliases.
- `requirements` - A list of image requirements with optional filters.

```{note}
The configuration file is always parsed from the last product version (alphabetically sorted).
```

Example for the distribution name:

```yaml
simplestream:
  distro_name: Ubuntu Core
```

Example for release aliases:

```yaml
simplestream:
  release_aliases:
    jammy: 22.04     # Single alias.
    noble: 24.04,24  # Multiple aliases.
```

Example for requirements:

```yaml
simplestream:
  requirements:

  # Applied to all images (no filters).
  - requirements:
      secure_boot: false

  # Applied to images that match the filters.
  - requirements:
      nesting: true
    releases:
    - noble
    architectures:
    - amd64
    variant:
    - default
    - desktop
```
