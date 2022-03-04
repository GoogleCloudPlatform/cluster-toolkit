variable "project_id" {
    type = string
}

variable "region" {
    type = string
}

variable "zone" {
    type = string
}

variable "subnet" {
    type = string
    default = ""
}

variable "static_ip" {
    type = string
    default = ""
}

variable "deployment_name" {
    description = "Base \"name\" for the deployment."
    type        = string
}

variable "webserver_hostname" {
    description = "DNS Hostname for the webserver"
    default     = ""
    type        = string
}

variable "django_su_username" {
    description = "DJango Admin SuperUser username"
    type        = string
    default     = "admin"
}

variable "django_su_password" {
    description = "DJango Admin SuperUser password"
    type        = string
    sensitive   = true
}

variable "django_su_email" {
    description = "DJango Admin SuperUser email"
    type        = string
}

variable "server_instance_type" {
    default = "e2-standard-2"
    type = string
}

variable "deployment_mode" {
    type = string
    description = "Use a tarball of this directory, or download from git to deploy the server. Must be either 'tarball' or 'git'"
    default = "tarball"
    validation {
        condition  =  var.deployment_mode == "tarball" || var.deployment_mode == "git"
        error_message = "The variable 'deployment_mode' must be either 'tarball' or 'git'."
    }
}

variable "ssh_key" {
    description = "admin SSH Key to add to the webserver instance"
    default = "~/.ssh/id_rsa.pub"
    type = string
}

variable "repo_branch" {
    default = "main"
    type = string
}

variable "repo_fork" {
    default = "GoogleCloudPlatform"
    type = string
}

variable "deployment_key" {
    default = ""
    type = string
}


variable "extra_labels" {
    type = map
    default = {}
}
