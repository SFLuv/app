-- Reverts the chain_id additions that the backend mistakenly applied to the
-- live Ponder database (migration 1.18 / boot-time PonderDB.BackfillTransactionChainIDs).
-- Those ALTERs changed Ponder's owned schema out from under the running indexer
-- and tripped its live-query triggers, halting indexing in production.
--
-- ORDERING: run this ONLY AFTER the reverted backend (no Ponder chain_id
-- backfill) and the reverted Ponder build (no chainId in ponder.schema.ts) are
-- deployed. Otherwise the old backend re-adds the column on its next boot.
--
-- Run against the Ponder database (PRODUCTION_POSTGRES_CONNECTION_STRING, db "ponder"):
--   psql "postgresql://USER:PASS@HOST:5432/ponder" -v ON_ERROR_STOP=1 -f drop_chain_id.sql
--
-- DROP COLUMN ... automatically drops the chain_id indexes the backfill created
-- (transfer_event_chain_*_idx, transfer_account_chain_address_idx,
--  allowance_chain_owner_spender_idx, approval_event_chain_*_idx).

BEGIN;

ALTER TABLE IF EXISTS transfer_event   DROP COLUMN IF EXISTS chain_id;
ALTER TABLE IF EXISTS transfer_account DROP COLUMN IF EXISTS chain_id;
ALTER TABLE IF EXISTS allowance        DROP COLUMN IF EXISTS chain_id;
ALTER TABLE IF EXISTS approval_event   DROP COLUMN IF EXISTS chain_id;

COMMIT;
