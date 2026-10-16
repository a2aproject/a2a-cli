#!/usr/bin/env bash
# Deletes what deploy.sh created: the Cloud Run service, the Artifact Registry
# repository with its images, the state bucket and the runtime service account.
#
#   PROJECT=my-project REGION=us-central1 deploy/relay/cloudrun/cleanup.sh [--yes]
#
# Takes the same PROJECT, REGION, SERVICE, BUCKET and REPO variables as
# deploy.sh, so set them the same way you did when deploying. The state bucket
# holds attached nodes, tokens and grants: deleting it means every agent has to
# be attached and paired again. Set KEEP_BUCKET=1 to keep it. APIs enabled by
# deploy.sh stay enabled, since other services may use them.
set -euo pipefail

command -v gcloud >/dev/null || { echo "gcloud is required" >&2; exit 1; }

PROJECT=${PROJECT:-$(gcloud config get-value project 2>/dev/null)}
[[ -n $PROJECT ]] || { echo "set PROJECT or a default gcloud project" >&2; exit 1; }
REGION=${REGION:-us-central1}
SERVICE=${SERVICE:-a2a-relay}
BUCKET=${BUCKET:-$PROJECT-a2a-relay-state}
REPO=${REPO:-a2a-relay}
KEEP_BUCKET=${KEEP_BUCKET:-}
sa="$SERVICE@$PROJECT.iam.gserviceaccount.com"

echo "This deletes, in project $PROJECT:"
echo "  Cloud Run service    $SERVICE ($REGION)"
echo "  Artifact Registry    $REPO ($REGION), with all its images"
if [[ -z $KEEP_BUCKET ]]; then
  echo "  Storage bucket       gs://$BUCKET, with the relay state"
fi
echo "  Service account      $sa"
if [[ ${1:-} != --yes ]]; then
  read -r -p "Continue? [y/N] " answer
  [[ $answer == [yY]* ]] || { echo "Aborted."; exit 1; }
fi

# delete NAME DESCRIBE-CMD -- DELETE-CMD: runs DELETE-CMD if DESCRIBE-CMD
# finds the resource, so a partial deploy or a second run is fine.
delete() {
  local name=$1; shift
  local describe=()
  while [[ $1 != -- ]]; do describe+=("$1"); shift; done
  shift
  if ! "${describe[@]}" >/dev/null 2>&1; then
    echo "$name: not found, skipping"
    return
  fi
  echo "$name: deleting"
  "$@"
}

delete "Cloud Run service $SERVICE" \
  gcloud run services describe "$SERVICE" --project "$PROJECT" --region "$REGION" -- \
  gcloud run services delete "$SERVICE" --project "$PROJECT" --region "$REGION" --quiet

delete "Artifact Registry repository $REPO" \
  gcloud artifacts repositories describe "$REPO" --project "$PROJECT" --location "$REGION" -- \
  gcloud artifacts repositories delete "$REPO" --project "$PROJECT" --location "$REGION" --quiet

if [[ -z $KEEP_BUCKET ]]; then
  delete "Bucket gs://$BUCKET" \
    gcloud storage buckets describe "gs://$BUCKET" --project "$PROJECT" -- \
    gcloud storage rm --recursive "gs://$BUCKET" --project "$PROJECT" --quiet
else
  echo "Bucket gs://$BUCKET: kept (KEEP_BUCKET is set)"
fi

delete "Service account $sa" \
  gcloud iam service-accounts describe "$sa" --project "$PROJECT" -- \
  gcloud iam service-accounts delete "$sa" --project "$PROJECT" --quiet

echo "Done."
