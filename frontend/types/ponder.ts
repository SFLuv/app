export type PonderSubscriptionType = "merchant" | "push"

export interface PonderSubscriptionRequest {
  address: string;
  email: string;
}

export interface PushSubscriptionSyncRequest {
  token: string;
  addresses: string[];
  /**
   * Legacy alias. Prefer preference_enabled and device_registered so app-level
   * preference and OS/device registration state stay distinct.
   */
  enabled?: boolean;
  preference_enabled?: boolean;
  device_registered?: boolean;
}

export interface PonderSubscription {
  id: number;
  address: string;
  type: PonderSubscriptionType;
  owner: string;
  data: string;
}

export interface PonderPushSubscription {
  id: number;
  owner: string;
  token: string;
  address: string;
  type: "push";
  data?: string;
  active: boolean;
  preference_enabled: boolean;
  device_registered: boolean;
  ponder_hook_id?: number;
}
