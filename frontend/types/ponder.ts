export type PonderSubscriptionType = "merchant"

export interface PonderSubscriptionRequest {
  address: string;
  email: string;
}

export interface PonderSubscription {
  id: number;
  address: string;
  type: PonderSubscriptionType;
  owner: string;
  data: string;
}