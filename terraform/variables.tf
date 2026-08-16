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

