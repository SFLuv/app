export interface LocationPaymentWallet {
  id: number;
  location_id: number;
  wallet_address: string;
  is_default: boolean;
}

export interface Location {
  id: number;
  google_id: string;
  name: string;
  pay_to_address?: string;
  tip_to_address?: string;
  description: string;
  type: string;
  street: string;
  city: string;
  state: string;
  zip: string;
  lat: number;
  lng: number;
  phone: string;
  email: string;
  website: string;
  image_url: string;
  rating: number;
  maps_page: string;
  opening_hours: string[];
}

export interface AuthedLocation {
  id: number;
  google_id: string;
  owner_id: string;
  name: string;
  description: string;
  type: string;
  approval?: boolean | null;
  street: string;
  city: string;
  state: string;
  zip: string;
  lat: number;
  lng: number;
  phone: string;
  email: string;
  admin_phone: string;
  admin_email: string;
  website: string;
  image_url: string;
  rating: number;
  maps_page: string;
  opening_hours: string[];
  contact_firstname: string;
  contact_lastname: string;
  contact_phone: string;
  pos_system: string;
  sole_proprietorship: string;
  tipping_policy: string;
  tipping_division: string;
  table_coverage: string;
  service_stations: number;
  tablet_model: string;
  messaging_service: string;
  pay_to_address?: string;
  tip_to_address?: string;
  payment_wallets: LocationPaymentWallet[];
  reference: string;
}

export interface GoogleSubLocation {
  google_id: string;
  name: string;
  type: string;
  street: string;
  city: string;
  state: string;
  zip: string;
  lat: number;
  lng: number;
  phone: string;
  website: string;
  image_url: string;
  rating: number;
  maps_page: string;
  opening_hours: string[];
}

export interface UpdateLocationApprovalRequest {
  id: number;
  approval: boolean | null;
}
