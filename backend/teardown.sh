#!/bin/bash

# Teardown script for Google Cloud resources
# This script will remove all resources created for the SFLuv application
set -e

# Configuration
PROJECT_ID="${PROJECT_ID:-sfluv-app}"
REGION="${REGION:-us-central1}"
CLUSTER_NAME="${CLUSTER_NAME:-sfluv-deployment-1-cluster}"
KSA_NAME="sfluv-ksa"
GSA_NAME="sfluv-gsa"
NAMESPACE="default"
REPOSITORY_NAME="sfluv-images"

echo "🗑️  SFLuv Google Cloud Teardown Script"
echo "======================================"
echo "⚠️  This will DELETE the following resources:"
echo "   • Kubernetes cluster: $CLUSTER_NAME"
echo "   • Google Service Account: $GSA_NAME"
echo "   • Secret Manager secrets: DB_PASSWORD, ADMIN_KEY, BOT_KEY, PRIVY_VKEY"
echo "   • Artifact Registry repository: $REPOSITORY_NAME"
echo "   • All deployed Kubernetes resources"
echo ""
echo "Project: $PROJECT_ID"
echo "Region: $REGION"
echo ""

# Confirmation prompt
read -p "Are you sure you want to proceed? This action cannot be undone. (yes/no): " confirm
if [[ $confirm != "yes" ]]; then
    echo "❌ Teardown cancelled."
    exit 0
fi

echo ""
echo "🚀 Starting teardown process..."

# 1. Delete Kubernetes resources first (if cluster exists)
echo "🧹 Cleaning up Kubernetes resources..."
if gcloud container clusters describe $CLUSTER_NAME --region=$REGION --project=$PROJECT_ID >/dev/null 2>&1; then
    echo "  • Deleting deployments..."
    kubectl delete deployment deployment-1 --ignore-not-found=true --timeout=60s
    
    echo "  • Deleting secrets..."
    kubectl delete secret sfluv-secrets --ignore-not-found=true
    
    echo "  • Deleting secret provider class..."
    kubectl delete secretproviderclass sfluv-secrets-spc --ignore-not-found=true
    
    echo "  • Deleting service account..."
    kubectl delete serviceaccount $KSA_NAME --namespace=$NAMESPACE --ignore-not-found=true
    
    echo "  • Waiting for resources to terminate..."
    sleep 10
else
    echo "  • Cluster $CLUSTER_NAME not found, skipping Kubernetes cleanup"
fi

# 2. Delete GKE Cluster
echo "🏗️  Deleting GKE cluster..."
if gcloud container clusters describe $CLUSTER_NAME --region=$REGION --project=$PROJECT_ID >/dev/null 2>&1; then
    gcloud container clusters delete $CLUSTER_NAME \
        --region=$REGION \
        --project=$PROJECT_ID \
        --quiet
    echo "  ✅ Cluster deleted"
else
    echo "  • Cluster $CLUSTER_NAME not found"
fi

# 3. Remove IAM policy binding
echo "🔗 Removing IAM policy bindings..."
if gcloud iam service-accounts describe $GSA_NAME@$PROJECT_ID.iam.gserviceaccount.com --project=$PROJECT_ID >/dev/null 2>&1; then
    echo "  • Removing workload identity binding..."
    gcloud iam service-accounts remove-iam-policy-binding \
        ${GSA_NAME}@${PROJECT_ID}.iam.gserviceaccount.com \
        --role roles/iam.workloadIdentityUser \
        --member "serviceAccount:${PROJECT_ID}.svc.id.goog[${NAMESPACE}/${KSA_NAME}]" \
        --project=$PROJECT_ID \
        --quiet || echo "    (binding may not exist)"
    
    echo "  • Removing project IAM binding..."
    gcloud projects remove-iam-policy-binding $PROJECT_ID \
        --member="serviceAccount:${GSA_NAME}@${PROJECT_ID}.iam.gserviceaccount.com" \
        --role="roles/secretmanager.secretAccessor" \
        --quiet || echo "    (binding may not exist)"
fi

# 4. Delete Google Service Account
echo "👤 Deleting Google Service Account..."
if gcloud iam service-accounts describe $GSA_NAME@$PROJECT_ID.iam.gserviceaccount.com --project=$PROJECT_ID >/dev/null 2>&1; then
    gcloud iam service-accounts delete ${GSA_NAME}@${PROJECT_ID}.iam.gserviceaccount.com \
        --project=$PROJECT_ID \
        --quiet
    echo "  ✅ Service account deleted"
else
    echo "  • Service account $GSA_NAME not found"
fi

# 5. Delete Secret Manager secrets
echo "🔐 Deleting Secret Manager secrets..."
secrets=("DB_PASSWORD" "ADMIN_KEY" "BOT_KEY" "PRIVY_VKEY")
for secret in "${secrets[@]}"; do
    if gcloud secrets describe $secret --project=$PROJECT_ID >/dev/null 2>&1; then
        gcloud secrets delete $secret --project=$PROJECT_ID --quiet
        echo "  ✅ Secret $secret deleted"
    else
        echo "  • Secret $secret not found"
    fi
done

# 6. Delete Artifact Registry repository
echo "📦 Deleting Artifact Registry repository..."
if gcloud artifacts repositories describe $REPOSITORY_NAME \
    --location=$REGION \
    --project=$PROJECT_ID >/dev/null 2>&1; then
    gcloud artifacts repositories delete $REPOSITORY_NAME \
        --location=$REGION \
        --project=$PROJECT_ID \
        --quiet
    echo "  ✅ Repository deleted"
else
    echo "  • Repository $REPOSITORY_NAME not found"
fi

# 7. Optional: Disable APIs (commented out by default to avoid affecting other projects)
echo "🔌 APIs will remain enabled (to avoid affecting other services)"
echo "   If you want to disable APIs manually:"
echo "   gcloud services disable secretmanager.googleapis.com --project=$PROJECT_ID"
echo "   gcloud services disable container.googleapis.com --project=$PROJECT_ID"
echo "   gcloud services disable artifactregistry.googleapis.com --project=$PROJECT_ID"

echo ""
echo "🎉 Teardown complete!"
echo ""
echo "📋 Summary of deleted resources:"
echo "   ✅ GKE Cluster: $CLUSTER_NAME"
echo "   ✅ Google Service Account: $GSA_NAME"
echo "   ✅ Secret Manager secrets: DB_PASSWORD, ADMIN_KEY, BOT_KEY, PRIVY_VKEY"
echo "   ✅ Artifact Registry repository: $REPOSITORY_NAME"
echo "   ✅ All Kubernetes deployments and resources"
echo ""
echo "💡 Note: Google Cloud billing will stop for these resources."
echo "   APIs remain enabled to avoid affecting other services."
echo ""
echo "🔍 To verify cleanup:"
echo "   gcloud container clusters list --project=$PROJECT_ID"
echo "   gcloud secrets list --project=$PROJECT_ID"
echo "   gcloud iam service-accounts list --project=$PROJECT_ID"
echo "   gcloud artifacts repositories list --project=$PROJECT_ID"