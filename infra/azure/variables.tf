variable "name" {
  description = "Prefix of every resource name."
  type        = string
  default     = "paddock-demo"
}

variable "location" {
  description = "Azure region."
  type        = string
  default     = "westeurope"
}

variable "image_registry" {
  description = "Where .github/workflows/publish-demo.yml pushed the images, without a trailing slash."
  type        = string
  default     = "ghcr.io/jassv9/apex-forge-paddock"
}

variable "image_tag" {
  description = "Tag of the core, engine, web-demo and demo-data images. Use a commit SHA to pin a deployment."
  type        = string
  default     = "demo"
}

variable "registry_username" {
  description = "User for a private registry. Leave empty when the images are public."
  type        = string
  default     = ""
}

variable "registry_password" {
  description = "Token for a private registry (for GHCR: read:packages). Pass it as TF_VAR_registry_password, never in a file."
  type        = string
  default     = ""
  sensitive   = true
}

variable "min_replicas" {
  description = "0 scales to zero when nobody is looking: no idle cost, and the first visitor waits about a minute while the session is ingested again. 1 keeps it warm."
  type        = number
  default     = 0

  validation {
    condition     = contains([0, 1], var.min_replicas)
    error_message = "min_replicas must be 0 or 1: every replica has its own database."
  }
}
