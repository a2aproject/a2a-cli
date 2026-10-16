#!/usr/bin/env bash
# Deploys an A2A relay (`a2a server relay serve`) to Cloud Run.
#
#   PROJECT=my-project REGION=us-central1 deploy/relay/cloudrun/deploy.sh
#
# The image is built locally with Docker and pushed to Artifact Registry, so
# no Cloud Build or build service account is involved.
#
# Attached nodes are kept in a Cloud Storage bucket mounted at /state; tasks
# are in memory, so the service runs as a single instance that is always on.
# The admin token is generated on the first deploy and kept on later ones,
# unless A2A_RELAY_ADMIN_TOKEN is set.
#
# Needs gcloud (logged in), docker, go and openssl. The relay must be reachable
# from the internet, so the project's org policies must allow public ingress
# and allUsers as Cloud Run invoker. See deploy/relay/README.md.
set -euo pipefail

for tool in gcloud docker go openssl; do
  command -v "$tool" >/dev/null || { echo "$tool is required" >&2; exit 1; }
done

PROJECT=${PROJECT:-$(gcloud config get-value project 2>/dev/null)}
[[ -n $PROJECT ]] || { echo "set PROJECT or a default gcloud project" >&2; exit 1; }
REGION=${REGION:-us-central1}
SERVICE=${SERVICE:-a2a-relay}
BUCKET=${BUCKET:-$PROJECT-a2a-relay-state}
REPO=${REPO:-a2a-relay}
root=$(cd "$(dirname "$0")/../../.." && pwd)
registry="$REGION-docker.pkg.dev"
image="$registry/$PROJECT/$REPO/relay:$(date +%Y%m%d-%H%M%S)"

gcloud services enable run.googleapis.com artifactregistry.googleapis.com iam.googleapis.com \
  storage.googleapis.com --project "$PROJECT"

# Build and push the image.
build=$(mktemp -d)
trap 'rm -rf "$build"' EXIT
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -C "$root" -trimpath -ldflags='-s -w' -o "$build/a2a" .
cp "$root/deploy/relay/cloudrun/Dockerfile" "$build/"
if ! gcloud artifacts repositories describe "$REPO" --project "$PROJECT" --location "$REGION" >/dev/null 2>&1; then
  gcloud artifacts repositories create "$REPO" --project "$PROJECT" --location "$REGION" --repository-format docker
fi
gcloud auth configure-docker "$registry" --quiet >/dev/null 2>&1
docker build --platform linux/amd64 -t "$image" "$build"
docker push "$image"

# A dedicated runtime service account, so the deploy doesn't depend on the
# default Compute Engine account, which many projects don't have.
sa="$SERVICE@$PROJECT.iam.gserviceaccount.com"
if ! gcloud iam service-accounts describe "$sa" --project "$PROJECT" >/dev/null 2>&1; then
  gcloud iam service-accounts create "$SERVICE" --project "$PROJECT" --display-name "A2A relay" >/dev/null
fi

if ! gcloud storage buckets describe "gs://$BUCKET" --project "$PROJECT" >/dev/null 2>&1; then
  gcloud storage buckets create "gs://$BUCKET" --project "$PROJECT" --location "$REGION" --uniform-bucket-level-access
fi
# A new service account takes a moment to become visible to IAM.
for i in 1 2 3 4 5 6; do
  gcloud storage buckets add-iam-policy-binding "gs://$BUCKET" --project "$PROJECT" \
    --member "serviceAccount:$sa" --role roles/storage.objectUser >/dev/null 2>&1 && break
  [[ $i == 6 ]] && { echo "could not grant $sa access to gs://$BUCKET" >&2; exit 1; }
  sleep $((i * 5))
done

env_flag=(--update-env-vars "A2A_RELAY_STATE_DIR=/state")
token=${A2A_RELAY_ADMIN_TOKEN:-}
if [[ -z $token ]] && ! gcloud run services describe "$SERVICE" --project "$PROJECT" --region "$REGION" >/dev/null 2>&1; then
  token=$(openssl rand -base64 24 | tr '+/' '-_' | tr -d '=')
fi
if [[ -n $token ]]; then
  env_flag=(--update-env-vars "A2A_RELAY_STATE_DIR=/state,A2A_RELAY_ADMIN_TOKEN=$token")
fi

gcloud run deploy "$SERVICE" --project "$PROJECT" --region "$REGION" --image "$image" --quiet \
  --service-account "$sa" --allow-unauthenticated --execution-environment gen2 \
  --min-instances 1 --max-instances 1 --no-cpu-throttling --timeout 3600 \
  --add-volume name=state,type=cloud-storage,bucket="$BUCKET" \
  --add-volume-mount volume=state,mount-path=/state \
  "${env_flag[@]}"

url=$(gcloud run services describe "$SERVICE" --project "$PROJECT" --region "$REGION" --format 'value(status.url)')
echo
echo "Relay: $url"
if [[ -n $token ]]; then
  echo "Admin token: $token"
  echo
  echo "  export A2A_RELAY_URL=$url A2A_RELAY_ADMIN_TOKEN=$token"
else
  echo "Admin token unchanged."
fi
echo
echo "Next, attach an agent's webhook (see deploy/relay/README.md):"
echo
echo "  a2a server relay attach <webhook-url> --name <agent> \\"
echo "    --webhook-header 'Authorization: Bearer <secret>' \\"
echo "    --relay $url --admin-token env:A2A_RELAY_ADMIN_TOKEN"
