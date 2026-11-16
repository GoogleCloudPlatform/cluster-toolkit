# modulewriter package

The resource writer (modulewriter) package writes various kinds of modules to
the deployment directory and ties them together with top-level deployment files,
for example the top level `main.tf` file for terraform modules. For local and
embedded Terraform modules that specify `reconfigure_ignores` in the blueprint,
it also writes an `override.tf.json` file merging the requested attributes into
each target resource's `lifecycle.ignore_changes` list.
