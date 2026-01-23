
export interface Event {
  id: string;
  title: string;
  description: string;
  amount: number;
  codes: number;
  expiration: number;
}

export type EventsStatus = "error" | "loading" | "ready"