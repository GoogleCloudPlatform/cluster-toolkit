### Submission Checklist

NOTE: Community submissions can take up to 2 weeks to be reviewed.

Please take the following actions before submitting this pull request.

* Fork your PR branch from the Toolkit "develop" branch (not main)
* Test all changes with pre-commit in a local branch [#](https://goo.gle/hpc-toolkit#development)
* Confirm that "make tests" passes all tests
* Add or modify unit tests to cover code changes
* Ensure that unit test coverage remains above 80%
* Update all applicable documentation
* Follow Cluster Toolkit Contribution guidelines [#](https://goo.gle/hpc-toolkit-contributing)

### Release impact

Does an existing cluster upgrade just by re-running `gcluster deploy` with the same blueprint? See [skills/release-breaking-check/SKILL.md](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/develop/skills/release-breaking-check/SKILL.md).

* [ ] Not breaking: nothing errors, is replaced or destroyed, or behaves differently for existing deployments.
* [ ] Breaking: label `release-breaking-changes` is added and both sections below are filled in.

#### Upgrade steps

<!-- Required if breaking. What must a user do before or after `gcluster deploy`? Must work for users skipping several versions. -->

#### Redeploy impact

<!-- Required if breaking. One line describing what happens on redeploy. Copied verbatim into the release notes. -->
