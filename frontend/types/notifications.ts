/**
 * The notification screen and the internal alert sets behind it.
 *
 * These mirror backend/structs/app_internal_notification.go. The split between
 * an ordinary notification and an internal alert lives in the data rather than
 * in two types, because both are rendered in one list: `internal` is present on
 * one and absent on the other.
 */

export type NotificationSetDefinition = {
  id: string
  label: string
  description: string
}

export type NotificationRecipient = {
  user_id: string
  name?: string
  email?: string
  role: string
  sets: string[]
}

export type NotificationSetsView = {
  catalog: NotificationSetDefinition[]
  recipients: NotificationRecipient[]
  can_edit: boolean
  /** How many unresolved alerts each set is currently holding. */
  live_counts: Record<string, number>
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
}
