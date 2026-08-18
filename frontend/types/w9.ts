/**
 * Tax and escrow types for the admin panel.
 *
 * The old W9Submission shape is gone with the system it described — it carried
 * a wallet, an email and a year, which was the entire "W9" the platform held.
 * The form itself now lives with a tax vendor, and nothing here ever sees a tax
 * identification number.
 */

/** One person's tax position for a year. */
export interface W9AdminRow {
  user_id: string
  contact_name?: string
  contact_email?: string
  tax_year: number
  filing_status: string
  earned_sfluv: string
  escrowed_sfluv: string
  back_pay_sfluv: string
  back_pay_count: number
  oldest_escrow_at?: string | null
  completed_at?: string | null
  /** True when money is queued and waiting on an admin to send it. */
  needs_back_pay_now: boolean
}

/**
 * Faucet coverage plus the per-person table.
 *
 * escrowed_sfluv is reserved and must not be allocated to anything else.
 * back_pay_sfluv is owed but deliberately not reserved: it only exists after an
 * escrow window lapsed and the money returned to the spendable pool, so an
 * admin funds it on purpose. Both are shown because approving back pay without
 * seeing them is a guess.
 */
export interface W9AdminOverview {
  faucet_sfluv: string
  allocated_sfluv: string
  escrowed_sfluv: string
  available_sfluv: string
  back_pay_sfluv: string
  back_pay_covered: boolean
  back_pay_short_by: string
  people_with_holds: number
  oldest_escrow_age_days: number
  rows: W9AdminRow[]
}
