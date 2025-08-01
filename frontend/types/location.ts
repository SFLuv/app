export interface Location {
  id: number;
  googleId: string;
  ownerId: string;
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
  imageUrl: string;
  rating: number;
  mapsPage: string;
}