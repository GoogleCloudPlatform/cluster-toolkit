# How to Contribute

We'd love to accept your patches and contributions to this project. There are
just a few small guidelines you need to follow.

## Branching Strategy
To maintain a stable release history, this repository follows a specific branching workflow:

`main`: Contains the latest stable release. This branch is only updated during a new release cycle.

`develop`: The active development branch. All new features, bug fixes, and community contributions are merged here first.

**Important**: All Pull Requests must be targeted at the `develop` branch. PRs targeting `main` will be closed or redirected.

## Contributor License Agreement

Contributions to this project must be accompanied by a Contributor License
Agreement. You (or your employer) retain the copyright to your contribution;
this simply gives us permission to use and redistribute your contributions as
part of the project. Head over to <https://cla.developers.google.com/> to see
your current agreements on file or to sign a new one.

You generally only need to submit a CLA once, so if you've already submitted one
(even if it was for a different project), you probably don't need to do it
again.

## Updating Dependencies

When updating the versions of Terraform or Packer used by the project, you must update the checksums to ensure secure downloads.

1. Update the `TERRAFORM_VERSION` and/or `PACKER_VERSION` variables in `tools/update-dependencies.sh`.
2. Run the `tools/update-dependencies.sh` script to fetch the latest SHA256 sums and generate `pkg/dependencies/checksums_generated.go`:

```shell
./tools/update-dependencies.sh
```

Ensure you commit both the modified script and the generated `pkg/dependencies/checksums_generated.go` file along with your pull request.

## Code Reviews

All submissions, including submissions by project members, require review. We
use GitHub pull requests for this purpose. Consult
[GitHub Help](https://help.github.com/articles/about-pull-requests/) for more
information on pull requests.

### Standard PR Response Times

Community submissions can take up to 2 weeks to be reviewed.

## Community Guidelines

This project follows [Google's Open Source Community
Guidelines](https://opensource.google/conduct/).
