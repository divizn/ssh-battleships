variable "region" {
  description = "where the instance, its volume and its parameters live"
  type        = string
  default     = "eu-west-2"
}

variable "name" {
  description = "prefixes every resource name, and is the Parameter Store path the instance reads"
  type        = string
  default     = "battleships"
}

variable "repo" {
  description = "cloned anonymously at boot, so it has to be public"
  type        = string
  default     = "https://github.com/divizn/ssh-battleships.git"
}

variable "open_cron" {
  description = "when the instance starts"
  type        = string
  default     = "cron(0 18 ? * FRI-SUN *)"
}

variable "close_cron" {
  description = "when the instance stops"
  type        = string
  default     = "cron(0 23 ? * FRI-SUN *)"
}

variable "timezone" {
  description = "named rather than an offset, so the hours hold across BST and GMT"
  type        = string
  default     = "Europe/London"
}
