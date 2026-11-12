# Runbook: the public read-only demo

What it is and why it is built this way: [ADR 0012](../adr/0012-read-only-demo.md).

**State: never deployed.** The steps under "Deploy to Azure" are written from the
provider's documentation and checked with `terraform validate`. Nobody has run them. Expect
to correct something on the first apply, and record it here when you do.

## Run it on this machine

Needs Docker. No game, no cloud account, no API key.

```bash
make demo-replica         # builds and starts the replica, http://localhost:5173
make demo-replica-check   # waits for the seeded diagnosis, then tests the door
make demo-replica-down    # stops it and deletes its volumes
```

The check prints one line per request and ends with
`the demo answers reads and refuses writes`. It takes about a minute after the images are
built. Port 5173 is the only one published; PostgreSQL, core and the engine are reachable
only from inside the replica. Stop `make dev` or `make demo` first: they use the same port.

If the check fails, `docker compose -f deploy/demo/docker-compose.replica.yml logs` shows
each container. Two lines of `ping database` errors from core at the start are expected:
core is restarted until PostgreSQL is up.

## Deploy to Azure

You need an Azure subscription, the Azure CLI, Terraform 1.9 or newer, and permission to
run workflows in this repository. Nothing here stores a credential in the repository.

1. **Publish the images.** On GitHub: Actions, "Publish demo images", Run workflow. It
   pushes `core`, `engine`, `web-demo` and `demo-data` to
   `ghcr.io/<owner>/<repository>/`, tagged `demo` and with the commit SHA.
2. **Let Azure pull them.** Either make the four packages public (GitHub, Packages, each
   package, Package settings, Change visibility), or keep them private and pass a token
   with `read:packages` in step 4.
3. **Sign in.**

   ```bash
   az login
   export ARM_SUBSCRIPTION_ID="<your subscription id>"
   ```

4. **Apply.**

   ```bash
   cd infra/azure
   terraform init
   terraform plan -out demo.tfplan     # read it: one resource group, logs, environment, app
   terraform apply demo.tfplan
   ```

   For private images add `-var registry_username=<github user>` and set
   `TF_VAR_registry_password` in the shell. To pin a deployment to a commit, add
   `-var image_tag=<sha>`.
5. **Check it.** `terraform output url` prints the address.

   ```bash
   sh deploy/demo/smoke.sh "$(terraform -chdir=infra/azure output -raw url)"
   ```

   The first request after an idle period starts a replica, which ingests and diagnoses the
   session again. Allow a few minutes.

Terraform keeps its state in `infra/azure/terraform.tfstate`, which holds the generated
database password. The file is ignored by git. Keep it private, or configure a remote
backend before anyone else needs to run this.

## Cost

With `min_replicas = 0` (the default) there is no replica while nobody visits, and the
running cost is the log workspace's ingestion. `-var min_replicas=1` keeps one replica of 2
vCPU and 4 Gi running all month, which is not free. These are the platform's published
rules, not a bill anyone has received for this project.

## Take it down

```bash
cd infra/azure
terraform destroy
```

Everything is in the one resource group that `terraform output resource_group` names.
There is no data to save.

## Change what the demo shows

The recorded sessions are `datasets/demo/spool`. Add a session there
(`datasets/demo/README.md`), publish the images again and apply with the new tag. Nothing
else holds state.
