# How to prune images hosted on the simple streams server

```
Usage:
  simplestream-maintainer prune <path> [flags]

Flags:
      --dangling                Remove dangling product versions (not referenced from a product catalog)
  -d, --image-dir strings       Image directory (relative to path argument) (default [images])
      --retain-builds int       Maximum number of product versions to retain (default 10)
      --retain-days int         Maximum number of days to retain any product version
      --stream-version string   Stream version (default "v1")
```

The prune command is used to remove no longer needed product versions (images).
Once pruning is complete, the product catalog and the simple streams index are updated accordingly.

## Retention policy

Product versions are retrieved from the existing product catalog and removed according to the set
retention policy.

The `--retain-builds` flag instructs `simplestream-maintainer` to keep the latest *n* versions
(sorted alphabetically) and remove everything else.

The `--retain-days` flag sets the maximum age of the product version and ensures that no product
version older than the specified number of days remains on the system or product catalog.
By default, this flag is set to `0` which means the product versions are not pruned by age.

## Dangling images

When pruning product versions, the stream's contents are retrieved from the product catalog. This means
there might exist an invalid product version that was not included in the product catalog (for
example, due to a missing metadata file or mismatched checksums). By default, such versions are
not removed.

The `--dangling` flag instructs `simplestream-maintainer` to remove product versions that are not
referenced by the product catalog. To ensure freshly uploaded or generated product versions are not
accidentally removed, unreferenced product versions are removed only if they are older than 6 hours.
