# Contributing

<!-- Include start contributing -->

The LXD team appreciates contributions to the project, through pull requests, issues on the [LXD GitHub repository](https://github.com/canonical/lxd/issues), or discussions or questions on the [forum](https://discourse.ubuntu.com/c/lxd/126).

Check the following guidelines before contributing to the project.

## Code of Conduct

When contributing, you must adhere to the Code of Conduct, which is available at: [`https://github.com/canonical/lxd-imagebuilder/blob/main/CODE_OF_CONDUCT.md`](https://github.com/canonical/lxd-imagebuilder/blob/main/CODE_OF_CONDUCT.md)

## License and copyright

All contributors must sign the [Canonical contributor license agreement](https://ubuntu.com/legal/contributors), which gives Canonical permission to use the contributions. The author of a change remains the copyright holder of their code (no copyright assignment).

By default, any contribution to this project is licensed out under the project license: AGPL-3.0-only.

By exception, Canonical may import code under licenses compatible with AGPL-3.0-only, such as Apache-2.0.
Such code will remain under its original license and will be identified as such in the commit message or its file header.

Some files and commits are licensed out under Apache-2.0 rather than AGPL-3.0-only.
These are marked as Apache-2.0 in their package-level COPYING file, file header or commit message.

## Pull requests

Changes to this project should be proposed as pull requests on GitHub
at: [`https://github.com/canonical/lxd-imagebuilder`](https://github.com/canonical/lxd-imagebuilder)

Proposed changes will then go through review there and once approved,
be merged in the main branch.

### Commit structure

Separate commits should be used for:

- Documentation (`doc: Update XYZ` for files in `doc/`)
- LXD imagebuilder (`lxd-imagebuilder: Add support for XYZ` for changes to `lxd-imagebuilder/`)
- Simplestream maintainer (`simplestream-maintainer: Add support for XYZ` for changes to `simplestream-maintainer/`)
- Tests (`tests: Add test for XYZ` for changes to `tests/`)

The same kind of pattern extends to the other tools in the LXD imagebuilder code tree
and depending on complexity, things may be split into even smaller chunks.

This structure makes it easier for contributions to be reviewed and also
greatly simplifies the process of back-porting fixes to stable branches.

### Developer Certificate of Origin

To improve tracking of contributions to this project we use the DCO 1.1
and use a "sign-off" procedure for all changes going into the branch.

The sign-off is a simple line at the end of the explanation for the
commit which certifies that you wrote it or otherwise have the right
to pass it on as an open-source contribution.

```
Developer Certificate of Origin
Version 1.1

Copyright (C) 2004, 2006 The Linux Foundation and its contributors.
660 York Street, Suite 102,
San Francisco, CA 94110 USA

Everyone is permitted to copy and distribute verbatim copies of this
license document, but changing it is not allowed.

Developer's Certificate of Origin 1.1

By making a contribution to this project, I certify that:

(a) The contribution was created in whole or in part by me and I
    have the right to submit it under the open source license
    indicated in the file; or

(b) The contribution is based upon previous work that, to the best
    of my knowledge, is covered under an appropriate open source
    license and I have the right under that license to submit that
    work with modifications, whether created in whole or in part
    by me, under the same open source license (unless I am
    permitted to submit under a different license), as indicated
    in the file; or

(c) The contribution was provided directly to me by some other
    person who certified (a), (b) or (c) and I have not modified
    it.

(d) I understand and agree that this project and the contribution
    are public and that a record of the contribution (including all
    personal information I submit with it, including my sign-off) is
    maintained indefinitely and may be redistributed consistent with
    this project or the open source license(s) involved.
```

An example of a valid sign-off line is:

```
Signed-off-by: Random J Developer <random@developer.org>
```

Use a known identity and a valid e-mail address.
Sorry, no anonymous contributions are allowed.

We also require each commit be individually signed-off by their author,
even when part of a larger set. You may find `git commit -s` useful.

<!-- Include end contributing -->
