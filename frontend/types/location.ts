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
  opening_hours: [number, number][]
}
