export interface Location {
  id: number;
  google_id: string;
  owner_id: string;
  name: string;
  description: string;
  type: string;
  approval: boolean;
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

}
