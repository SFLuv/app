/**
 * The notification screen and the notification hub behind it.
 *
 * These mirror backend/structs/app_internal_notification.go and
 * backend/structs/notification_hub.go. The split between an ordinary
 * notification and an internal one lives in the data rather than in two types,
 * because both are rendered in one list: `internal` is present on one and
 * absent on the other.
 */

/** One group topics are shown under: support, admin, dev, activity. */
export type NotificationSetDefinition = {
  id: string
  label: string
  description: string
}

/** One thing the hub can be told about. */
export type NotificationTopicDefinition = {
  id: string
  /** The group it is shown under, a NotificationSetDefinition id. */
  set: string
  label: string
  description: string
}

/** A group of house-organization members, and how they want to be told. */
export type NotificationTeam = {
  id: number
  name: string
  description: string
  /** Members always see the team's notifications in the app; these are extras. */
  push_members: boolean
  email_members: boolean
  email_address: string
  chat_conversation_id?: number | null
  /** The group chat's title, filled in by the backend for display. */
  chat_name?: string
  member_ids: string[]
  /** Topic ids routed to this team, including "unrouted" for the catch-all. */
  topics: string[]
  created_at: string
}

/** The body of a team create or update. Every field is replaced. */
export type NotificationTeamInput = {
  name: string
  description: string
  push_members: boolean
  email_members: boolean
  email_address: string
  chat_conversation_id: number | null
}

/** A house-organization member, as the team editor lists them. */
export type NotificationHubMember = {
  user_id: string
  name?: string
  email?: string
  role: string
  /** Phones a push can reach. */
  devices: number
}

export type NotificationDeliveryTeam = {
  id: number
  name: string
}

/** Where one notification went. */
export type NotificationDelivery = {
  teams: NotificationDeliveryTeam[] | null
  /** The topic had no team and the catch-all carried it instead. */
  catch_all?: boolean
  recipients: number
  pushed: number
  emailed?: string[]
  chats?: number[]
  failures?: string[]
  reached_nobody: boolean
}

export type NotificationResolution = "" | "acted" | "acked" | "cleared" | "info"

/** One notification in the hub's recent list. */
export type NotificationHubEntry = {
  id: number
  topic: string
  topic_label?: string
  set: string
  kind: string
  title: string
  body: string
  created_at: string
  resolved_at?: string | null
  resolution?: NotificationResolution
  outcome?: string
  /** Absent on notifications from before deliveries were recorded. */
  delivery?: NotificationDelivery | null
}

/** GET /organizations/mine/notification-hub */
export type NotificationHubView = {
  sets: NotificationSetDefinition[]
  topics: NotificationTopicDefinition[]
  teams: NotificationTeam[]
  members: NotificationHubMember[]
  /** Topic id to team ids, the catch-all under "unrouted". Absent topics have no team. */
  routes: Record<string, number[]>
  /** Unresolved notifications per topic. */
  live_counts: Record<string, number>
  /** Newest first. */
  recent: NotificationHubEntry[]
  /** Notifications in the last 30 days that reached no person, address or chat. */
  reached_nobody: number
  can_edit: boolean
  /** False while the backend cannot receive email, so email topics never fire. */
  inbound_email: boolean
  /** Every way in and the topics each posts under, for the visualizer. */
  touchpoints?: NotificationTouchpointDefinition[]
  /** Per topic: when it last fired, how many in a day, how many reached nobody in 30 days. */
  topic_activity?: Record<string, NotificationTopicActivity>
  /** When the backend read this view. */
  generated_at?: string
}

/** One way somebody reaches us, or one thing the platform raises, and its topics. */
export type NotificationTouchpointDefinition = {
  id: string
  group: string
  label: string
  topics: string[]
  description: string
  /** Outside our systems, so it cannot reach the hub. */
  outside?: boolean
}

export type NotificationTopicActivity = {
  last_at?: string
  last_24h: number
  reached_nobody_30d: number
}

export type NotificationField = {
  label: string
  value: string
}

export type NotificationActionButton = {
  id: string
  label: string
  style?: "default" | "primary" | "danger"
  /** Present on a link button the client opens itself, like a mailto reply. */
  url?: string
  /** When set, ask this before running. */
  confirm?: string
}

export type InternalNotificationDetail = {
  id: number
  set: string
  set_label: string
  /** What it is about; the unit notifications are given to teams by. */
  topic?: string
  topic_label?: string
  alert_key: string
  fields?: NotificationField[]
  urgent: boolean
  updated_at: string
  /** Empty for an alert that retires itself — a low balance has no button. */
  actions?: NotificationActionButton[]
  resolved: boolean
  resolved_at?: string
  /** The history line: "Approved by Dana", "Back up after 4 minutes". */
  outcome?: string
}

export type AppNotification = {
  id: number
  kind: string
  title: string
  body: string
  created_at: string
  data?: Record<string, string>
  seen: boolean
  internal?: InternalNotificationDetail
}

export type NotificationFeed = {
  active: AppNotification[]
  history: AppNotification[]
  next_cursor?: string
  unseen_count: number
  sets: string[]
  /** Names of the notification teams this user is on. */
  teams?: string[]
}
