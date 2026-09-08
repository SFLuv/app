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
  oldest_escrow_at?: string | null
  completed_at?: string | null

  /**
   * Vestigial. Back pay was retired with the tier redesign — a hold can no
   * longer accumulate, because the payment after one is refused rather than
   * held, so no money can lapse into being owed-but-unreserved. The backend
   * still returns these and they are now always zero/false. Kept only so the
   * response continues to type-check; delete on both sides together.
   */
  back_pay_sfluv: string
  back_pay_count: number
  needs_back_pay_now: boolean
}

/**
 * Faucet coverage plus the per-person table.
 *
 * escrowed_sfluv is reserved and must not be allocated to anything else, which
 * is why it is subtracted from what is available. The back_pay_* fields below
 * are vestigial — see W9AdminRow.
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
