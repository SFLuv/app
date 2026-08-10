export interface LocationHoursInterval {
  open_minute: number;
  close_minute: number;
}

export interface LocationDayHours {
  weekday: number;
  is_closed: boolean;
  /** A day can open more than once — lunch and dinner is the ordinary case. */
  intervals: LocationHoursInterval[];
}

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
  hours?: LocationDayHours[];
  hours_manual?: boolean;
  hours_synced_at?: string | null;
}

/**
 * Which onboarding path produced a location. `google_place` is the primary
 * route and the only one whose name and address Google vouches for;
 * `manual` exists for businesses with no Google Business Profile, where the
 * merchant types the name themselves and only the address is autocompleted.
 */
export type ListingSource = "google_place" | "manual";

export interface AuthedLocation {
  id: number;
  google_id: string;
  listing_source?: ListingSource;
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
  hours?: LocationDayHours[];
  hours_manual?: boolean;
  hours_synced_at?: string | null;
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
  hours?: LocationDayHours[];
  hours_manual?: boolean;
  hours_synced_at?: string | null;
  /** Raw Places types, used to reject results that are postal addresses rather than businesses. */
  types?: string[];
  /** Google's own one-line address, shown back to the merchant for confirmation. */
  formatted_address?: string;
}

/**
 * An address picked from Google's geocode autocomplete when the business has no
 * Google listing of its own. Deliberately has no `name`: the whole failure mode
 * this path guards against is a merchant ending up named after their street, so
 * the name has to be typed separately rather than inherited from the address.
 */
export interface ManualAddressDraft {
  street: string;
  city: string;
  state: string;
  zip: string;
  lat: number;
  lng: number;
  formatted_address?: string;
}

export type PlaceSelection =
  | { source: "google_place"; place: GoogleSubLocation }
  | { source: "manual"; address: ManualAddressDraft };

export interface UpdateLocationApprovalRequest {
  id: number;
  approval: boolean | null;
}
