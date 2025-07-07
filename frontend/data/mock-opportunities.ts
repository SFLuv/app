import type { Opportunity } from "@/types/opportunity"

export const mockOrganizers = [
  "SF Food Bank",
  "Golden Gate Park Conservancy",
  "Habitat for Humanity SF",
  "SPCA San Francisco",
  "Clean SF Initiative",
  "SF Community Gardens",
  "Homeless Outreach Team",
  "Youth Mentorship Program",
]

export const mockOpportunities: Opportunity[] = [
  {
    id: "opp-1",
    title: "Community Garden Maintenance",
    description:
      "Help maintain our community gardens by weeding, planting, and general upkeep. No experience necessary, tools and guidance provided. This is a great opportunity to learn about urban gardening while helping keep our community spaces beautiful and productive.",
    date: "2025-05-15T10:00:00",
    organizer: "SF Community Gardens",
    location: {
      address: "123 Garden St",
      city: "San Francisco",
      state: "CA",
      zip: "94110",
      coordinates: {
        lat: 37.7599,
        lng: -122.4148,
      },
    },
    rewardAmount: 50,
    volunteersNeeded: 10,
    volunteersSignedUp: 6,
    imageUrl: "/placeholder.svg?height=200&width=400",
  },
  {
    id: "opp-2",
    title: "Food Bank Sorting",
    description:
      "Help sort and package food donations at the SF Food Bank. This vital work helps ensure that food gets distributed efficiently to those in need throughout the city. Training provided on-site.",
    date: "2025-05-18T09:00:00",
    organizer: "SF Food Bank",
    location: {
      address: "900 Pennsylvania Ave",
      city: "San Francisco",
      state: "CA",
      zip: "94107",
      coordinates: {
        lat: 37.7544,
        lng: -122.3921,
      },
    },
    rewardAmount: 45,
    volunteersNeeded: 20,
    volunteersSignedUp: 15,
    imageUrl: "/placeholder.svg?height=200&width=400",
  },
  {
    id: "opp-3",
    title: "Park Cleanup Day",
    description:
      "Join us for our monthly park cleanup day. We'll be picking up trash, clearing paths, and making our parks more beautiful for everyone. Gloves and tools provided. Great for families and groups!",
    date: "2025-05-20T08:00:00",
    organizer: "Golden Gate Park Conservancy",
    location: {
      address: "501 Stanyan St",
      city: "San Francisco",
      state: "CA",
      zip: "94117",
      coordinates: {
        lat: 37.7694,
        lng: -122.4862,
      },
    },
    rewardAmount: 40,
    volunteersNeeded: 30,
    volunteersSignedUp: 12,
    imageUrl: "/placeholder.svg?height=200&width=400",
  },
  {
    id: "opp-4",
    title: "Animal Shelter Assistant",
    description:
      "Help care for animals at the SPCA shelter. Tasks include walking dogs, socializing cats, cleaning cages, and assisting with adoption events. Training provided for all volunteers.",
    date: "2025-05-22T13:00:00",
    organizer: "SPCA San Francisco",
    location: {
      address: "201 Alabama St",
      city: "San Francisco",
      state: "CA",
      zip: "94103",
      coordinates: {
        lat: 37.7651,
        lng: -122.4121,
      },
    },
    rewardAmount: 55,
    volunteersNeeded: 15,
    volunteersSignedUp: 10,
    imageUrl: "/placeholder.svg?height=200&width=400",
  },
  {
    id: "opp-5",
    title: "Homeless Outreach",
    description:
      "Join our outreach team distributing essentials to homeless individuals. We'll be providing food, hygiene kits, and information about available services. Training session required before participation.",
    date: "2025-05-25T17:00:00",
    organizer: "Homeless Outreach Team",
    location: {
      address: "149 Turk St",
      city: "San Francisco",
      state: "CA",
      zip: "94102",
      coordinates: {
        lat: 37.7825,
        lng: -122.4132,
      },
    },
    rewardAmount: 60,
    volunteersNeeded: 12,
    volunteersSignedUp: 5,
    imageUrl: "/placeholder.svg?height=200&width=400",
  },
  {
    id: "opp-6",
    title: "Beach Cleanup",
    description:
      "Help keep our beaches clean and safe for wildlife and visitors. We'll be collecting trash and microplastics along Ocean Beach. Supplies provided, but bring sunscreen and water!",
    date: "2025-05-27T09:00:00",
    organizer: "Clean SF Initiative",
    location: {
      address: "Great Highway",
      city: "San Francisco",
      state: "CA",
      zip: "94121",
      coordinates: {
        lat: 37.7691,
        lng: -122.5107,
      },
    },
    rewardAmount: 45,
    volunteersNeeded: 25,
    volunteersSignedUp: 18,
    imageUrl: "/placeholder.svg?height=200&width=400",
  },
  {
    id: "opp-7",
    title: "Youth Tutoring",
    description:
      "Provide academic support to underserved youth in our after-school program. Subjects include math, science, reading, and writing. Minimum commitment of 2 hours per week for 8 weeks.",
    date: "2025-05-29T15:00:00",
    organizer: "Youth Mentorship Program",
    location: {
      address: "711 Van Ness Ave",
      city: "San Francisco",
      state: "CA",
      zip: "94102",
      coordinates: {
        lat: 37.7819,
        lng: -122.4212,
      },
    },
    rewardAmount: 65,
    volunteersNeeded: 10,
    volunteersSignedUp: 4,
    imageUrl: "/placeholder.svg?height=200&width=400",
  },
  {
    id: "opp-8",
    title: "Home Building Project",
    description:
      "Help build affordable housing with Habitat for Humanity. No construction experience necessary - we'll teach you everything you need to know! Lunch and tools provided.",
    date: "2025-06-01T08:00:00",
    organizer: "Habitat for Humanity SF",
    location: {
      address: "1050 Tennessee St",
      city: "San Francisco",
      state: "CA",
      zip: "94107",
      coordinates: {
        lat: 37.7575,
        lng: -122.3885,
      },
    },
    rewardAmount: 70,
    volunteersNeeded: 15,
    volunteersSignedUp: 9,
    imageUrl: "/placeholder.svg?height=200&width=400",
  },
  {
    id: "opp-9",
    title: "Senior Companion",
    description:
      "Spend time with seniors at our community center. Activities include conversation, games, arts and crafts, and assistance with technology. Make a difference in the lives of our elderly community members.",
    date: "2025-06-03T10:00:00",
    organizer: "SF Community Gardens",
    location: {
      address: "890 Beach St",
      city: "San Francisco",
      state: "CA",
      zip: "94109",
      coordinates: {
        lat: 37.8058,
        lng: -122.4225,
      },
    },
    rewardAmount: 50,
    volunteersNeeded: 8,
    volunteersSignedUp: 3,
    imageUrl: "/placeholder.svg?height=200&width=400",
  },
  {
    id: "opp-10",
    title: "Tree Planting Day",
    description:
      "Help increase San Francisco's urban forest by planting trees throughout the city. Learn about native species and proper planting techniques. Great for environmental enthusiasts!",
    date: "2025-06-05T09:00:00",
    organizer: "Clean SF Initiative",
    location: {
      address: "501 Stanyan St",
      city: "San Francisco",
      state: "CA",
      zip: "94117",
      coordinates: {
        lat: 37.7694,
        lng: -122.4862,
      },
    },
    rewardAmount: 55,
    volunteersNeeded: 20,
    volunteersSignedUp: 7,
    imageUrl: "/placeholder.svg?height=200&width=400",
  },
  {
    id: "opp-11",
    title: "Mural Painting Project",
    description:
      "Join local artists in creating a community mural. No artistic experience necessary - there are tasks for all skill levels. Help beautify our neighborhood with public art!",
    date: "2025-06-08T11:00:00",
    organizer: "Youth Mentorship Program",
    location: {
      address: "24th St & Mission St",
      city: "San Francisco",
      state: "CA",
      zip: "94110",
      coordinates: {
        lat: 37.7525,
        lng: -122.4186,
      },
    },
    rewardAmount: 60,
    volunteersNeeded: 15,
    volunteersSignedUp: 8,
    imageUrl: "/placeholder.svg?height=200&width=400",
  },
  {
    id: "opp-12",
    title: "Farmers Market Assistant",
    description:
      "Help local farmers set up and run their stands at the weekly farmers market. Tasks include setup, customer assistance, and breakdown. Learn about local food systems while helping small producers.",
    date: "2025-06-10T07:00:00",
    organizer: "SF Food Bank",
    location: {
      address: "Ferry Building",
      city: "San Francisco",
      state: "CA",
      zip: "94111",
      coordinates: {
        lat: 37.7955,
        lng: -122.3937,
      },
    },
    rewardAmount: 45,
    volunteersNeeded: 10,
    volunteersSignedUp: 6,
    imageUrl: "/placeholder.svg?height=200&width=400",
  },
]
