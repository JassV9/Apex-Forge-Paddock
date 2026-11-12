# The public, read-only demo on Azure Container Apps (docs/adr/0012-read-only-demo.md).
#
# One container app, one replica, five containers that share localhost: PostgreSQL, the
# engine, core, the dashboard behind nginx, and the seeder. Only nginx is reachable from
# outside, and it lets only reads through. Nothing is persistent: a new replica ingests the
# recorded session again, so there is no state to back up, migrate or leak.
#
# Checked with `terraform validate` only: this has never been applied. The same layout runs
# locally as `make demo-replica` (deploy/demo/docker-compose.replica.yml).

locals {
  images = {
    core   = "${var.image_registry}/core:${var.image_tag}"
    engine = "${var.image_registry}/engine:${var.image_tag}"
    web    = "${var.image_registry}/web-demo:${var.image_tag}"
    data   = "${var.image_registry}/demo-data:${var.image_tag}"
  }
  registry_host = split("/", var.image_registry)[0]
  # Scratch space shared between containers of the replica.
  volumes = ["spool", "data", "outbox"]
}

resource "azurerm_resource_group" "demo" {
  name     = "${var.name}-rg"
  location = var.location
}

resource "azurerm_log_analytics_workspace" "demo" {
  name                = "${var.name}-logs"
  location            = azurerm_resource_group.demo.location
  resource_group_name = azurerm_resource_group.demo.name
  sku                 = "PerGB2018"
  retention_in_days   = 30
}

resource "azurerm_container_app_environment" "demo" {
  name                       = "${var.name}-env"
  location                   = azurerm_resource_group.demo.location
  resource_group_name        = azurerm_resource_group.demo.name
  log_analytics_workspace_id = azurerm_log_analytics_workspace.demo.id
}

# The database listens only inside the replica. It still gets a password of its own, made
# here and kept in the app's secrets, never in a file.
resource "random_password" "database" {
  length  = 32
  special = false
}

resource "azurerm_container_app" "demo" {
  name                         = var.name
  resource_group_name          = azurerm_resource_group.demo.name
  container_app_environment_id = azurerm_container_app_environment.demo.id
  revision_mode                = "Single"

  secret {
    name  = "database-password"
    value = random_password.database.result
  }

  secret {
    name  = "database-url"
    value = "postgres://paddock:${random_password.database.result}@localhost:5432/paddock?sslmode=disable"
  }

  dynamic "secret" {
    for_each = var.registry_username == "" ? [] : [1]
    content {
      name  = "registry-password"
      value = var.registry_password
    }
  }

  dynamic "registry" {
    for_each = var.registry_username == "" ? [] : [1]
    content {
      server               = local.registry_host
      username             = var.registry_username
      password_secret_name = "registry-password"
    }
  }

  ingress {
    external_enabled = true
    target_port      = 8088

    traffic_weight {
      latest_revision = true
      percentage      = 100
    }
  }

  template {
    min_replicas = var.min_replicas
    max_replicas = 1 # every replica has its own database

    dynamic "volume" {
      for_each = local.volumes
      content {
        name         = volume.value
        storage_type = "EmptyDir"
      }
    }

    # Copies the recorded session into the spool and opens the volumes core writes to.
    init_container {
      name  = "dataset"
      image = local.images.data

      dynamic "volume_mounts" {
        for_each = local.volumes
        content {
          name = volume_mounts.value
          path = "/${volume_mounts.value}"
        }
      }
    }

    # The five containers ask for 2 vCPU and 4 Gi together, the most a replica gets on the
    # smallest plan, in the platform's steps of 0.25 vCPU with 2 Gi per vCPU.
    container {
      name   = "postgres"
      image  = "postgres:17-alpine"
      cpu    = 0.25
      memory = "0.5Gi"

      env {
        name  = "POSTGRES_USER"
        value = "paddock"
      }
      env {
        name  = "POSTGRES_DB"
        value = "paddock"
      }
      env {
        name        = "POSTGRES_PASSWORD"
        secret_name = "database-password"
      }
    }

    container {
      name   = "engine"
      image  = local.images.engine
      cpu    = 0.75
      memory = "1.5Gi"

      env {
        name  = "PADDOCK_LLM_PROVIDER"
        value = "none" # template text: the demo calls no hosted model and holds no key
      }

      volume_mounts {
        name = "data"
        path = "/data"
      }
    }

    container {
      name   = "core"
      image  = local.images.core
      cpu    = 0.5
      memory = "1Gi"

      env {
        name  = "PADDOCK_HTTP_ADDR"
        value = ":8080"
      }
      env {
        name        = "PADDOCK_DATABASE_URL"
        secret_name = "database-url"
      }
      env {
        name  = "PADDOCK_ENGINE_URL"
        value = "http://localhost:8000"
      }
      env {
        name  = "PADDOCK_SPOOL_DIR"
        value = "/spool"
      }
      env {
        name  = "PADDOCK_DATA_DIR"
        value = "/data"
      }
      env {
        name  = "PADDOCK_OUTBOX_DIR"
        value = "/outbox"
      }

      dynamic "volume_mounts" {
        for_each = local.volumes
        content {
          name = volume_mounts.value
          path = "/${volume_mounts.value}"
        }
      }
    }

    container {
      name   = "web"
      image  = local.images.web
      cpu    = 0.25
      memory = "0.5Gi"

      env {
        name  = "PADDOCK_WEB_PORT"
        value = "8088"
      }
      env {
        name  = "PADDOCK_CORE_URL"
        value = "http://localhost:8080"
      }
    }

    # Diagnoses the recorded session once, from inside: visitors can only read.
    container {
      name    = "seed"
      image   = "curlimages/curl:latest"
      cpu     = 0.25
      memory  = "0.5Gi"
      command = ["/bin/sh", "-c"]
      args    = [file("${path.module}/../../deploy/demo/seed.sh")]

      env {
        name  = "PADDOCK_CORE_URL"
        value = "http://localhost:8080"
      }
      env {
        name  = "PADDOCK_SEED_HOLD"
        value = "1"
      }
    }
  }
}
