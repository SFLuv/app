#!/bin/bash

# Setup script for Google Cloud Secret Manager integration with GKE
set -e

# Configuration
PROJECT_ID="${PROJECT_ID:-sfluv-app}"
REGION="${REGION:-us-central1}"
CLUSTER_NAME="${CLUSTER_NAME:-sfluv-deployment-1-cluster}"
KSA_NAME="sfluv-ksa"
GSA_NAME="sfluv-gsa"
NAMESPACE="default"

echo "🔐 Setting up Google Secret Manager integration for GKE..."
echo "Project: $PROJECT_ID"
echo "Region: $REGION" 
echo "Cluster: $CLUSTER_NAME"

# 1. Enable required APIs
echo "🔌 Enabling required Google Cloud APIs..."
gcloud services enable secretmanager.googleapis.com --project=$PROJECT_ID
gcloud services enable container.googleapis.com --project=$PROJECT_ID

# 2. Enable Secret Manager on GKE cluster
echo "🔧 Enabling Secret Manager on GKE cluster..."
gcloud container clusters update $CLUSTER_NAME \
    --region=$REGION \
    --enable-secret-manager \
    --project=$PROJECT_ID

# 3. Create Google Service Account (if not exists)
echo "👤 Creating Google Service Account..."
if ! gcloud iam service-accounts describe $GSA_NAME@$PROJECT_ID.iam.gserviceaccount.com --project=$PROJECT_ID >/dev/null 2>&1; then
    gcloud iam service-accounts create $GSA_NAME \
        --display-name="SFLuv GKE Service Account" \
        --description="Service account for SFLuv application to access secrets" \
        --project=$PROJECT_ID
else
    echo "Google Service Account $GSA_NAME already exists"
fi

# 4. Grant Secret Manager permissions
echo "🗝️ Granting Secret Manager permissions..."
gcloud projects add-iam-policy-binding $PROJECT_ID \
    --member="serviceAccount:${GSA_NAME}@${PROJECT_ID}.iam.gserviceaccount.com" \
    --role="roles/secretmanager.secretAccessor"

# 5. Create Kubernetes Service Account and bind to Google Service Account
echo "🎭 Setting up Kubernetes Service Account..."
kubectl create serviceaccount $KSA_NAME \
    --namespace=$NAMESPACE \
    --dry-run=client -o yaml | kubectl apply -f -

kubectl annotate serviceaccount $KSA_NAME \
    --namespace=$NAMESPACE \
    iam.gke.io/gcp-service-account=${GSA_NAME}@${PROJECT_ID}.iam.gserviceaccount.com \
    --overwrite

# 6. Bind the Kubernetes Service Account to Google Service Account
echo "🔗 Binding service accounts for Workload Identity..."
gcloud iam service-accounts add-iam-policy-binding \
    ${GSA_NAME}@${PROJECT_ID}.iam.gserviceaccount.com \
    --role roles/iam.workloadIdentityUser \
    --member "serviceAccount:${PROJECT_ID}.svc.id.goog[${NAMESPACE}/${KSA_NAME}]" \
    --project=$PROJECT_ID

# 7. Function to create regional secret if it doesn't exist
create_regional_secret_if_not_exists() {
    local secret_name=$1
    local secret_value=$2
    
    if ! gcloud secrets describe $secret_name --project=$PROJECT_ID >/dev/null 2>&1; then
        echo "Creating regional secret: $secret_name"
        echo -n "$secret_value" | gcloud secrets create $secret_name \
            --replication-policy=user-managed \
            --locations=$REGION \
            --data-file=- \
            --project=$PROJECT_ID
    else
        echo "Secret $secret_name already exists"
    fi
}

echo "📝 Creating regional secrets in Google Secret Manager..."

# Check if required environment variables are set
if [[ -z "$DB_PASSWORD" || -z "$ADMIN_KEY" || -z "$BOT_KEY" || -z "$PRIVY_VKEY" ]]; then
    echo "❌ Error: Required environment variables are not set."
    echo "Please set the following environment variables before running this script:"
    echo "  export DB_PASSWORD='your_db_password'"
    echo "  export ADMIN_KEY='your_admin_key'"
    echo "  export BOT_KEY='your_bot_key'"
    echo "  export PRIVY_VKEY='your_privy_verification_key'"
    echo ""
    echo "Alternatively, you can source your .env file:"
    echo "  source .env.production && ./setup-secrets.sh"
    exit 1
fi

echo "✅ Using environment variables for secret values"

# Create regional secrets with values from environment variables
create_regional_secret_if_not_exists "DB_PASSWORD" "$DB_PASSWORD"
create_regional_secret_if_not_exists "ADMIN_KEY" "$ADMIN_KEY"
create_regional_secret_if_not_exists "BOT_KEY" "$BOT_KEY"
create_regional_secret_if_not_exists "PRIVY_VKEY" "$PRIVY_VKEY"

# 8. Apply SecretProviderClass
echo "🚀 Applying SecretProviderClass..."
kubectl apply -f secret-provider-class.yaml

echo ""
echo "✅ Setup complete!"
echo ""
echo "📋 Usage:"
echo "To use this script, set the required environment variables first:"
echo "  export DB_PASSWORD='your_actual_db_password'"
echo "  export ADMIN_KEY='your_actual_admin_key'"
echo "  export BOT_KEY='your_actual_bot_key'"
echo "  export PRIVY_VKEY='your_actual_privy_verification_key'"
echo "  ./setup-secrets.sh"
echo ""
echo "Or source your .env file:"
echo "  source .env.production && ./setup-secrets.sh"
echo ""
echo "📋 Next steps:"
echo "1. Deploy your application:"
echo "   kubectl apply -f deployment-1.yaml"
echo ""
echo "2. Check deployment status:"
echo "   kubectl get pods -l app=deployment-1"
echo ""
echo "3. Verify secrets are mounted:"
echo "   kubectl exec -it deployment/deployment-1 -- ls -la /mnt/secrets"
echo ""
echo "4. Check application logs:"
echo "   kubectl logs deployment/deployment-1"