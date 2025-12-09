#!/bin/bash

# Build and deploy script for Google Kubernetes Engine
set -e

# Configuration
PROJECT_ID="${PROJECT_ID:-your-gcp-project-id}"
REGION="${REGION:-us-central1}"
CLUSTER_NAME="${CLUSTER_NAME:-sfluv-cluster}"
IMAGE_NAME="gcr.io/${PROJECT_ID}/sfluv-backend"
IMAGE_TAG="${IMAGE_TAG:-$(git rev-parse --short HEAD)}"

echo "🚀 Starting GKE deployment..."
echo "Project: $PROJECT_ID"
echo "Region: $REGION"
echo "Cluster: $CLUSTER_NAME"
echo "Image: $IMAGE_NAME:$IMAGE_TAG"

# 1. Build the Docker image
echo "📦 Building Docker image..."
docker build -f Dockerfile.gke -t $IMAGE_NAME:$IMAGE_TAG .
docker tag $IMAGE_NAME:$IMAGE_TAG $IMAGE_NAME:latest

# 2. Configure Docker for GCR
echo "🔐 Configuring Docker for Google Container Registry..."
gcloud auth configure-docker

# 3. Push to Google Container Registry
echo "📤 Pushing image to GCR..."
docker push $IMAGE_NAME:$IMAGE_TAG
docker push $IMAGE_NAME:latest

# 4. Get GKE credentials
echo "🔑 Getting GKE credentials..."
gcloud container clusters get-credentials $CLUSTER_NAME --region=$REGION --project=$PROJECT_ID

# 5. Create secrets and configmaps
echo "🗝️ Creating Kubernetes secrets..."
kubectl create namespace sfluv --dry-run=client -o yaml | kubectl apply -f -

# Create secrets (you'll need to provide actual values)
kubectl create secret generic sfluv-secrets \
  --from-literal=db-url="postgres://user:password@host:5432/dbname?sslmode=require" \
  --from-literal=admin-key="${ADMIN_KEY}" \
  --from-literal=bot-key="${BOT_KEY}" \
  --from-literal=privy-vkey="${PRIVY_VKEY}" \
  --namespace=sfluv \
  --dry-run=client -o yaml | kubectl apply -f -

# Create configmap
kubectl create configmap sfluv-config \
  --from-literal=privy-app-id="${PRIVY_APP_ID}" \
  --from-literal=rpc-url="${RPC_URL}" \
  --from-literal=token-id="${TOKEN_ID}" \
  --from-literal=token-decimals="${TOKEN_DECIMALS}" \
  --namespace=sfluv \
  --dry-run=client -o yaml | kubectl apply -f -

# 6. Update deployment with new image
echo "🚢 Deploying to Kubernetes..."
sed "s|gcr.io/YOUR_PROJECT_ID|gcr.io/${PROJECT_ID}|g" k8s-deployment.yaml | \
sed "s|:latest|:${IMAGE_TAG}|g" | \
kubectl apply -f -

# 7. Wait for deployment to complete
echo "⏳ Waiting for deployment to complete..."
kubectl rollout status deployment/sfluv-backend -n sfluv --timeout=300s

# 8. Get service info
echo "✅ Deployment complete!"
echo ""
echo "Service information:"
kubectl get services -n sfluv
echo ""
echo "Pod information:"
kubectl get pods -n sfluv
echo ""
echo "Ingress information:"
kubectl get ingress -n sfluv

echo ""
echo "🎉 Deployment successful!"
echo "Your application should be available at: https://api.sfluv.org"