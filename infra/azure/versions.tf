terraform {
  required_version = ">= 1.9"

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 4.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }
}

# The subscription comes from the environment (ARM_SUBSCRIPTION_ID) or `az login`; nothing
# that identifies an account is written in this repository.
provider "azurerm" {
  features {}
}
