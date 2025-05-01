resource "terraform_data" "input_validation" {
  lifecycle {
    precondition {
      condition = (
        var.repo_password == null ||
        (var.use_upstream_credentials && var.repo_mode == "REMOTE_REPOSITORY")
      )
      error_message = "repo_password may be set only when repo_mode=REMOTE_REPOSITORY and use_upstream_credentials=true."
    }

    precondition {
      condition = (
        !var.use_upstream_credentials ||
        var.repo_mode == "REMOTE_REPOSITORY"
      )
      error_message = "use_upstream_credentials is allowed only when repo_mode is REMOTE_REPOSITORY."
    }

    precondition {
      condition = (
        var.repo_mode != "REMOTE_REPOSITORY" ||
        (var.repo_public_repository != null || var.repo_mirror_url != null)
      )
      error_message = "For a REMOTE_REPOSITORY you must set repo_public_repository or repo_mirror_url."
    }

    precondition {
      condition = (
        !contains(["APT", "YUM"], var.format) ||
        (var.repository_base != null && var.repository_path != null)
      )
      error_message = "APT/YUM formats require repository_base and repository_path."
    }
  }
}
