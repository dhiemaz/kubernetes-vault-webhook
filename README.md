# Kubernetes Vault Webhook

Kubernetes Mutation WebHook that adds an in memory volume to all containers and a init container that grabs secrets from vault and places them into a file.  I have only just started this project and it is in a basic working state but there is still a way to go before its useable. 

The plan is to support multiple file formats like key value, JSON, XML and YAML.  Maybe I will support consul templates later on too.  Right now only key value is working which can be sourced or exported into environment variables using sh or bash.

The idea behind this project is to keep it simple as possible and let the application consume the secrets and let the developer choose whether just to source the file into environment variables or allow the application to consume the secrets as json or yaml file.

There are two parts to this project.  The webhook that mutates the pod to include the init container and volume and the vault-init cli tool that gets called from the init container and pulls the secrets down.

I will update the readme with better descriopns and instructions once I get this into a responsible state. 


## Deploying

move into the hack directory
```
./create_certs.sh --service vault-webhook --secret webhook-certs
echo -n $(kubectl config view --raw --flatten -o json | jq -r '.clusters[] | select(.name == "'$(kubectl config current-context)'") | .cluster."certificate-authority-data"');echo
```
Move into the deployment directory and replace the CA_CERT variable on webhook.yml with the output of the last command and save and apply the Kubernetes manifests.
```
kubectl apply -f .
```

### Webhook Environment Variables

The following environment variables configure the webhook
* `WEBHOOK_DEFAULT_ADDRESS`
 The default vault server to pull secrets from. 

* `WEBHOOK_METRICS_PORT`
   The port to listen on for prometheus to scrape metrics from

* `WEBHOOK_INIT_CONTAINER_CPU_REQUESTS`
  The Init Container CPU Requests
  
* `WEBHOOK_INIT_CONTAINER_CPU_LIMITS`
 The Init Container CPU Limits

* `WEBHOOK_INIT_CONTAINER_MEMORY_REQUEST`
    The Init Container Ram Requests

* `WEBHOOK_INIT_CONTAINER_MEMORY_LIMITS``
  The Init Container Ram Limits

* `WEBHOOK_VAULT_INIT_IMAGE`
  The container to use to pull vault secrets down with


