package db

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

// ErrNoProtocolSigner means the account has no live signing key, which is
// different from the account not existing: it happens between minting a Safe's
// first key and the chain accepting it.
var ErrNoProtocolSigner = errors.New("protocol account has no active signer")

// ── The registry ─────────────────────────────────────────────────────────────

// EnsureProtocolAccount registers a protocol function, leaving an existing row
// alone apart from its label and description.
//
// The address and the signer are deliberately not touched on conflict. Those
// are earned state — a deployed Safe and the key that owns it — and a boot that
// re-declared them would hand the account a new identity every restart.
func (a *AppDB) EnsureProtocolAccount(ctx context.Context, account structs.ProtocolAccount) error {
	if _, err := a.db.Exec(ctx, `
		INSERT INTO protocol_accounts(key, label, description, role, chain_id, owner_index)
		VALUES($1, $2, $3, $4, $5, $6)
		ON CONFLICT (key) DO UPDATE
		SET label = EXCLUDED.label,
			description = EXCLUDED.description,
			role = EXCLUDED.role,
			updated_at = NOW();
	`, account.Key, account.Label, account.Description, account.Role, account.ChainID, account.OwnerIndex); err != nil {
		return fmt.Errorf("error registering protocol account %s: %w", account.Key, err)
	}
	return nil
}

const protocolAccountColumns = `
	a.key, a.label, a.description, a.role, a.chain_id,
	COALESCE(a.safe_address, ''), a.owner_index,
	EXTRACT(EPOCH FROM a.deployed_at)::bigint, a.deploy_tx,
	EXTRACT(EPOCH FROM a.created_at)::bigint,
	EXTRACT(EPOCH FROM a.updated_at)::bigint`

func scanProtocolAccount(row pgx.Row) (*structs.ProtocolAccount, error) {
	account := &structs.ProtocolAccount{}
	if err := row.Scan(
		&account.Key, &account.Label, &account.Description, &account.Role, &account.ChainID,
		&account.SafeAddress, &account.OwnerIndex,
		&account.DeployedAt, &account.DeployTx,
		&account.CreatedAt, &account.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return account, nil
}

func (a *AppDB) GetProtocolAccount(ctx context.Context, key string) (*structs.ProtocolAccount, error) {
	account, err := scanProtocolAccount(a.db.QueryRow(ctx,
		`SELECT`+protocolAccountColumns+` FROM protocol_accounts a WHERE a.key = $1;`, key))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("error loading protocol account %s: %w", key, err)
	}
	signer, err := a.GetActiveProtocolSigner(ctx, key)
	if err != nil && !errors.Is(err, ErrNoProtocolSigner) {
		return nil, err
	}
	if signer != nil {
		account.Signer = &signer.ProtocolSigner
	}
	return account, nil
}

// ListProtocolAccounts returns every account on a chain with its active signer
// attached, which is what the panel renders.
func (a *AppDB) ListProtocolAccounts(ctx context.Context, chainID int) ([]structs.ProtocolAccount, error) {
	rows, err := a.db.Query(ctx,
		`SELECT`+protocolAccountColumns+` FROM protocol_accounts a WHERE a.chain_id = $1 ORDER BY a.key;`, chainID)
	if err != nil {
		return nil, fmt.Errorf("error listing protocol accounts: %w", err)
	}
	defer rows.Close()

	accounts := []structs.ProtocolAccount{}
	for rows.Next() {
		account, err := scanProtocolAccount(rows)
		if err != nil {
			return nil, fmt.Errorf("error scanning a protocol account: %w", err)
		}
		accounts = append(accounts, *account)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range accounts {
		signer, err := a.GetActiveProtocolSigner(ctx, accounts[i].Key)
		if err != nil && !errors.Is(err, ErrNoProtocolSigner) {
			return nil, err
		}
		if signer != nil {
			accounts[i].Signer = &signer.ProtocolSigner
		}
	}
	return accounts, nil
}

// SetProtocolAccountSafe records the address an account's Safe will have, or
// already has.
//
// Refuses to move an address that is already set. The Safe address is derived
// from the owner key and the factory salt, so a second answer means something
// upstream changed its mind about one of those — and silently repointing would
// orphan whatever funds are at the first address.
func (a *AppDB) SetProtocolAccountSafe(ctx context.Context, key, safeAddress string) error {
	tag, err := a.db.Exec(ctx, `
		UPDATE protocol_accounts
		SET safe_address = $2, updated_at = NOW()
		WHERE key = $1
		AND (safe_address IS NULL OR LOWER(safe_address) = LOWER($2));
	`, key, safeAddress)
	if err != nil {
		return fmt.Errorf("error recording the Safe address for %s: %w", key, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf(
			"protocol account %s already has a different Safe address recorded; refusing to repoint it",
			key,
		)
	}
	return nil
}

// MarkProtocolAccountDeployed stamps the deployment, once.
func (a *AppDB) MarkProtocolAccountDeployed(ctx context.Context, key, txHash string) error {
	if _, err := a.db.Exec(ctx, `
		UPDATE protocol_accounts
		SET deployed_at = COALESCE(deployed_at, NOW()),
			deploy_tx = CASE WHEN deploy_tx = '' THEN $2 ELSE deploy_tx END,
			updated_at = NOW()
		WHERE key = $1;
	`, key, txHash); err != nil {
		return fmt.Errorf("error marking %s deployed: %w", key, err)
	}
	return nil
}

// ── Signers ──────────────────────────────────────────────────────────────────

// SealedProtocolSigner carries the encrypted key alongside the public record.
// Only the protocol service opens it; nothing else should ever ask for this
// type, which is why the sealed blob is not on structs.ProtocolSigner.
type SealedProtocolSigner struct {
	structs.ProtocolSigner
	SealedKey string
}

func (a *AppDB) GetActiveProtocolSigner(ctx context.Context, accountKey string) (*SealedProtocolSigner, error) {
	signer := &SealedProtocolSigner{}
	err := a.db.QueryRow(ctx, `
		SELECT id, account_key, eoa_address, sealed_key,
			EXTRACT(EPOCH FROM created_at)::bigint,
			EXTRACT(EPOCH FROM activated_at)::bigint,
			activation_tx,
			EXTRACT(EPOCH FROM retired_at)::bigint,
			retired_reason, COALESCE(rotated_by, '')
		FROM protocol_account_signers
		WHERE account_key = $1 AND activated_at IS NOT NULL AND retired_at IS NULL;
	`, accountKey).Scan(
		&signer.ID, &signer.AccountKey, &signer.EOAAddress, &signer.SealedKey,
		&signer.CreatedAt, &signer.ActivatedAt, &signer.ActivationTx,
		&signer.RetiredAt, &signer.RetiredReason, &signer.RotatedBy,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNoProtocolSigner
		}
		return nil, fmt.Errorf("error loading the active signer for %s: %w", accountKey, err)
	}
	return signer, nil
}

// InsertProtocolSigner records a freshly minted key, not yet live.
func (a *AppDB) InsertProtocolSigner(ctx context.Context, accountKey, eoaAddress, sealedKey, rotatedBy string) (int64, error) {
	var id int64
	by := strings.TrimSpace(rotatedBy)
	var byValue any
	if by != "" {
		byValue = by
	}
	if err := a.db.QueryRow(ctx, `
		INSERT INTO protocol_account_signers(account_key, eoa_address, sealed_key, rotated_by)
		VALUES($1, $2, $3, $4)
		RETURNING id;
	`, accountKey, eoaAddress, sealedKey, byValue).Scan(&id); err != nil {
		return 0, fmt.Errorf("error recording a new signer for %s: %w", accountKey, err)
	}
	return id, nil
}

// PromoteProtocolSigner makes a minted key the live one and retires whatever
// held the role before it, in one transaction.
//
// Both halves together or neither. Activating without retiring would trip the
// one-active-signer index and leave the account unreadable; retiring without
// activating would leave it with no signer at all, which is worse — the Safe
// would be fine and the backend would have forgotten how to sign for it.
func (a *AppDB) PromoteProtocolSigner(ctx context.Context, accountKey string, signerID int64, activationTx, retiredReason string) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error beginning a signer promotion: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE protocol_account_signers
		SET retired_at = NOW(), retired_reason = $3
		WHERE account_key = $1 AND id <> $2 AND retired_at IS NULL AND activated_at IS NOT NULL;
	`, accountKey, signerID, retiredReason); err != nil {
		return fmt.Errorf("error retiring the previous signer for %s: %w", accountKey, err)
	}

	tag, err := tx.Exec(ctx, `
		UPDATE protocol_account_signers
		SET activated_at = COALESCE(activated_at, NOW()), activation_tx = $3
		WHERE id = $2 AND account_key = $1 AND retired_at IS NULL;
	`, accountKey, signerID, activationTx)
	if err != nil {
		return fmt.Errorf("error activating the new signer for %s: %w", accountKey, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("signer %d is not an unretired signer of %s", signerID, accountKey)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("error committing a signer promotion: %w", err)
	}
	return nil
}

// AbandonProtocolSigner retires a key that was minted but never took, so a
// failed rotation does not leave a row that looks like a pending one forever.
func (a *AppDB) AbandonProtocolSigner(ctx context.Context, signerID int64, reason string) error {
	if _, err := a.db.Exec(ctx, `
		UPDATE protocol_account_signers
		SET retired_at = NOW(), retired_reason = $2
		WHERE id = $1 AND activated_at IS NULL AND retired_at IS NULL;
	`, signerID, reason); err != nil {
		return fmt.Errorf("error abandoning signer %d: %w", signerID, err)
	}
	return nil
}

// ListProtocolSigners is the key history for one account, newest first.
func (a *AppDB) ListProtocolSigners(ctx context.Context, accountKey string) ([]structs.ProtocolSigner, error) {
	rows, err := a.db.Query(ctx, `
		SELECT id, account_key, eoa_address,
			EXTRACT(EPOCH FROM created_at)::bigint,
			EXTRACT(EPOCH FROM activated_at)::bigint,
			activation_tx,
			EXTRACT(EPOCH FROM retired_at)::bigint,
			retired_reason, COALESCE(rotated_by, '')
		FROM protocol_account_signers
		WHERE account_key = $1
		ORDER BY created_at DESC, id DESC;
	`, accountKey)
	if err != nil {
		return nil, fmt.Errorf("error listing signers for %s: %w", accountKey, err)
	}
	defer rows.Close()

	signers := []structs.ProtocolSigner{}
	for rows.Next() {
		var signer structs.ProtocolSigner
		if err := rows.Scan(
			&signer.ID, &signer.AccountKey, &signer.EOAAddress,
			&signer.CreatedAt, &signer.ActivatedAt, &signer.ActivationTx,
			&signer.RetiredAt, &signer.RetiredReason, &signer.RotatedBy,
		); err != nil {
			return nil, fmt.Errorf("error scanning a signer: %w", err)
		}
		signers = append(signers, signer)
	}
	return signers, rows.Err()
}

// ── Address history: the part that makes an old check answerable ─────────────

// OpenProtocolAddressWindow records that an address holds a role from a given
// moment. Idempotent: re-declaring an address that already holds the role is
// the common case on boot and must not open a second window.
func (a *AppDB) OpenProtocolAddressWindow(
	ctx context.Context,
	address, role string,
	chainID int,
	accountKey string,
	startedAt time.Time,
	source, note string,
) error {
	var key any
	if trimmed := strings.TrimSpace(accountKey); trimmed != "" {
		key = trimmed
	}
	if _, err := a.db.Exec(ctx, `
		INSERT INTO protocol_address_history(address, role, chain_id, account_key, started_at, source, note)
		SELECT $1, $2, $3, $4, $5, $6, $7
		WHERE NOT EXISTS (
			SELECT 1 FROM protocol_address_history
			WHERE role = $2 AND chain_id = $3 AND LOWER(address) = LOWER($1) AND ended_at IS NULL
		);
	`, strings.ToLower(strings.TrimSpace(address)), role, chainID, key, startedAt, source, note); err != nil {
		return fmt.Errorf("error opening an address window for %s as %s: %w", address, role, err)
	}
	return nil
}

// CloseProtocolAddressWindow ends an address's turn at a role.
//
// endedAt is passed rather than defaulted to NOW() because the moment that
// matters is the cutover, not the moment somebody got round to recording it. A
// window closed an hour late would attribute an hour of the new Safe's
// payments to the old address.
func (a *AppDB) CloseProtocolAddressWindow(ctx context.Context, address, role string, chainID int, endedAt time.Time) error {
	if _, err := a.db.Exec(ctx, `
		UPDATE protocol_address_history
		SET ended_at = $4
		WHERE role = $2 AND chain_id = $3 AND LOWER(address) = LOWER($1) AND ended_at IS NULL;
	`, strings.ToLower(strings.TrimSpace(address)), role, chainID, endedAt); err != nil {
		return fmt.Errorf("error closing the address window for %s as %s: %w", address, role, err)
	}
	return nil
}

// ProtocolAddressesAt answers "which addresses held this role at this moment".
//
// A set rather than one address, for two reasons: a cutover leaves both the old
// and the new address live for the instant between the windows, and a role can
// legitimately have more than one holder (several admin wallets have been paid
// from). A caller comparing a transfer's sender wants "was it any of ours",
// which is a membership test.
func (a *AppDB) ProtocolAddressesAt(ctx context.Context, role string, chainID int, at time.Time) ([]string, error) {
	rows, err := a.db.Query(ctx, `
		SELECT LOWER(address)
		FROM protocol_address_history
		WHERE role = $1
		AND chain_id = $2
		AND started_at <= $3
		AND (ended_at IS NULL OR ended_at > $3)
		ORDER BY started_at;
	`, role, chainID, at)
	if err != nil {
		return nil, fmt.Errorf("error resolving %s addresses at %s: %w", role, at.Format(time.RFC3339), err)
	}
	defer rows.Close()

	addresses := []string{}
	for rows.Next() {
		var address string
		if err := rows.Scan(&address); err != nil {
			return nil, fmt.Errorf("error scanning a historical address: %w", err)
		}
		addresses = append(addresses, address)
	}
	return addresses, rows.Err()
}

// CurrentProtocolAddresses is the open windows for a role.
func (a *AppDB) CurrentProtocolAddresses(ctx context.Context, role string, chainID int) ([]string, error) {
	rows, err := a.db.Query(ctx, `
		SELECT LOWER(address)
		FROM protocol_address_history
		WHERE role = $1 AND chain_id = $2 AND ended_at IS NULL
		ORDER BY started_at;
	`, role, chainID)
	if err != nil {
		return nil, fmt.Errorf("error listing current %s addresses: %w", role, err)
	}
	defer rows.Close()

	addresses := []string{}
	for rows.Next() {
		var address string
		if err := rows.Scan(&address); err != nil {
			return nil, fmt.Errorf("error scanning a current address: %w", err)
		}
		addresses = append(addresses, address)
	}
	return addresses, rows.Err()
}

// ListProtocolAddressHistory is every window on a chain, for the panel.
func (a *AppDB) ListProtocolAddressHistory(ctx context.Context, chainID int) ([]structs.ProtocolAddressWindow, error) {
	rows, err := a.db.Query(ctx, `
		SELECT id, LOWER(address), role, chain_id, COALESCE(account_key, ''),
			EXTRACT(EPOCH FROM started_at)::bigint,
			EXTRACT(EPOCH FROM ended_at)::bigint,
			source, note
		FROM protocol_address_history
		WHERE chain_id = $1
		ORDER BY role, started_at DESC, id DESC;
	`, chainID)
	if err != nil {
		return nil, fmt.Errorf("error listing protocol address history: %w", err)
	}
	defer rows.Close()

	windows := []structs.ProtocolAddressWindow{}
	for rows.Next() {
		var window structs.ProtocolAddressWindow
		if err := rows.Scan(
			&window.ID, &window.Address, &window.Role, &window.ChainID, &window.AccountKey,
			&window.StartedAt, &window.EndedAt, &window.Source, &window.Note,
		); err != nil {
			return nil, fmt.Errorf("error scanning an address window: %w", err)
		}
		windows = append(windows, window)
	}
	return windows, rows.Err()
}

// ── The cutover ledger ───────────────────────────────────────────────────────

// ClaimProtocolStep marks a step running and reports whether this caller is the
// one that may run it.
//
// The claim is the whole point. These steps move money with keys that are
// presumed exposed, and a boot loop or a second instance must not repeat one.
// An insert that conflicts only advances to 'running' from a state that is not
// already done, so a completed step is never re-entered.
func (a *AppDB) ClaimProtocolStep(ctx context.Context, step string) (bool, error) {
	var claimed bool
	err := a.db.QueryRow(ctx, `
		INSERT INTO protocol_migration_steps(step, status, attempts, started_at, updated_at)
		VALUES($1, 'running', 1, NOW(), NOW())
		ON CONFLICT (step) DO UPDATE
		SET status = 'running',
			attempts = protocol_migration_steps.attempts + 1,
			updated_at = NOW()
		WHERE protocol_migration_steps.status IN ('pending', 'failed')
		RETURNING TRUE;
	`, step).Scan(&claimed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The conflict target existed and was already done, running or
			// skipped. Not an error — just not ours to run.
			return false, nil
		}
		return false, fmt.Errorf("error claiming cutover step %s: %w", step, err)
	}
	return claimed, nil
}

// FinishProtocolStep records the outcome of a claimed step.
func (a *AppDB) FinishProtocolStep(ctx context.Context, step, status, detail, txHash string, amount *big.Int) error {
	var amountValue any
	if amount != nil {
		amountValue = amount.String()
	}
	completed := status == structs.ProtocolStepDone || status == structs.ProtocolStepSkipped
	if _, err := a.db.Exec(ctx, `
		UPDATE protocol_migration_steps
		SET status = $2,
			detail = $3,
			tx_hash = CASE WHEN $4 = '' THEN tx_hash ELSE $4 END,
			amount_base = COALESCE($5::numeric, amount_base),
			updated_at = NOW(),
			completed_at = CASE WHEN $6 THEN NOW() ELSE completed_at END
		WHERE step = $1;
	`, step, status, detail, txHash, amountValue, completed); err != nil {
		return fmt.Errorf("error finishing cutover step %s: %w", step, err)
	}
	return nil
}

// ProtocolStepStatus reports a step's recorded status, or "" when it has never
// run. Used to decide whether a dependent step may proceed.
func (a *AppDB) ProtocolStepStatus(ctx context.Context, step string) (string, error) {
	var status string
	err := a.db.QueryRow(ctx, `SELECT status FROM protocol_migration_steps WHERE step = $1;`, step).Scan(&status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("error reading cutover step %s: %w", step, err)
	}
	return status, nil
}

// ListProtocolSteps is the ledger, for the panel.
func (a *AppDB) ListProtocolSteps(ctx context.Context) ([]structs.ProtocolMigrationStep, error) {
	rows, err := a.db.Query(ctx, `
		SELECT step, status, detail, tx_hash,
			COALESCE(amount_base::text, ''), attempts,
			EXTRACT(EPOCH FROM started_at)::bigint,
			EXTRACT(EPOCH FROM updated_at)::bigint,
			EXTRACT(EPOCH FROM completed_at)::bigint
		FROM protocol_migration_steps
		ORDER BY started_at, step;
	`)
	if err != nil {
		return nil, fmt.Errorf("error listing cutover steps: %w", err)
	}
	defer rows.Close()

	steps := []structs.ProtocolMigrationStep{}
	for rows.Next() {
		var step structs.ProtocolMigrationStep
		if err := rows.Scan(
			&step.Step, &step.Status, &step.Detail, &step.TxHash,
			&step.AmountBase, &step.Attempts,
			&step.StartedAt, &step.UpdatedAt, &step.CompletedAt,
		); err != nil {
			return nil, fmt.Errorf("error scanning a cutover step: %w", err)
		}
		steps = append(steps, step)
	}
	return steps, rows.Err()
}

// GetSealedProtocolSigner loads one signer row by id, including its sealed key.
//
// Needed for the pending signer during deployment and rotation: the key that is
// about to own a Safe is not the account's active signer yet, so it cannot be
// reached through GetActiveProtocolSigner.
func (a *AppDB) GetSealedProtocolSigner(ctx context.Context, id int64) (*SealedProtocolSigner, error) {
	signer := &SealedProtocolSigner{}
	err := a.db.QueryRow(ctx, `
		SELECT id, account_key, eoa_address, sealed_key,
			EXTRACT(EPOCH FROM created_at)::bigint,
			EXTRACT(EPOCH FROM activated_at)::bigint,
			activation_tx,
			EXTRACT(EPOCH FROM retired_at)::bigint,
			retired_reason, COALESCE(rotated_by, '')
		FROM protocol_account_signers
		WHERE id = $1;
	`, id).Scan(
		&signer.ID, &signer.AccountKey, &signer.EOAAddress, &signer.SealedKey,
		&signer.CreatedAt, &signer.ActivatedAt, &signer.ActivationTx,
		&signer.RetiredAt, &signer.RetiredReason, &signer.RotatedBy,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("signer %d does not exist", id)
		}
		return nil, fmt.Errorf("error loading signer %d: %w", id, err)
	}
	return signer, nil
}

// ── Which transaction was which operation ────────────────────────────────────

// ProtocolTransaction is one transaction we know we caused.
type ProtocolTransaction struct {
	TxHash string
	// LogIndex identifies which transfer within the transaction this is.
	// Batched payouts share a transaction, so the hash alone does not identify
	// one payout's money.
	LogIndex      uint
	ChainID       int
	AccountKey    string
	SenderAddress string
	SignerAddress string
	OperationHash string
	Purpose       string
	Recipient     string
	AmountBase    *big.Int
	BlockNumber   uint64
}

// Purposes, for the panel and for reading the table by hand.
const (
	ProtocolTxPurposePayout   = "faucet_payout"
	ProtocolTxPurposeDeploy   = "deploy"
	ProtocolTxPurposeRotation = "rotation"
	ProtocolTxPurposeRole     = "role_grant"
	ProtocolTxPurposeSweep    = "sweep"
)

// ClaimProtocolTransaction records a transaction as ours, and reports whether
// this caller is the one that got it.
//
// False means some other operation already owns that hash. That is the whole
// point: two payouts of the same amount to the same person in the same minute
// must not both record the same transfer, and the loser keeps looking for one
// of its own rather than taking credit for work it did not do.
func (a *AppDB) ClaimProtocolTransaction(ctx context.Context, record ProtocolTransaction) (bool, error) {
	hash := strings.ToLower(strings.TrimSpace(record.TxHash))
	if hash == "" {
		return false, fmt.Errorf("no transaction hash to claim")
	}
	var amount any
	if record.AmountBase != nil {
		amount = record.AmountBase.String()
	}
	var block any
	if record.BlockNumber > 0 {
		block = int64(record.BlockNumber)
	}

	var claimed bool
	err := a.db.QueryRow(ctx, `
		INSERT INTO protocol_transactions(
			tx_hash, log_index, chain_id, account_key, sender_address, signer_address,
			operation_hash, purpose, recipient, amount_base, block_number
		)
		VALUES($1, $11, $2, $3, $4, $5, $6, $7, $8, $9::numeric, $10)
		ON CONFLICT (tx_hash, log_index) DO NOTHING
		RETURNING TRUE;
	`,
		hash, record.ChainID, record.AccountKey,
		strings.ToLower(strings.TrimSpace(record.SenderAddress)),
		strings.ToLower(strings.TrimSpace(record.SignerAddress)),
		record.OperationHash, record.Purpose,
		strings.ToLower(strings.TrimSpace(record.Recipient)),
		amount, block, int(record.LogIndex),
	).Scan(&claimed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("error claiming transaction %s: %w", hash, err)
	}
	return claimed, nil
}

// ProtocolTransactionSender answers "which of our addresses sent this
// transaction", by lookup rather than by inference.
//
// Empty means we have no record of it, which is not the same as it not being
// ours: everything sent before this table existed is absent from it, which is
// why the caller falls back to the address windows rather than treating a miss
// as a foreign transaction.
func (a *AppDB) ProtocolTransactionSender(ctx context.Context, txHash string) (string, string, bool, error) {
	var sender, accountKey string
	// Any row for the hash answers it: every transfer in a batched transaction
	// was sent by the same account, which is the only thing asked here.
	err := a.db.QueryRow(ctx, `
		SELECT LOWER(sender_address), account_key
		FROM protocol_transactions
		WHERE tx_hash = LOWER($1)
		LIMIT 1;
	`, strings.TrimSpace(txHash)).Scan(&sender, &accountKey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", false, nil
		}
		return "", "", false, fmt.Errorf("error looking up transaction %s: %w", txHash, err)
	}
	return sender, accountKey, true, nil
}

// ProtocolTransactionClaimed reports whether a hash is already spoken for,
// without claiming it. Used while scanning candidate transfers.
func (a *AppDB) ProtocolTransactionClaimed(ctx context.Context, txHash string) (bool, error) {
	var exists bool
	if err := a.db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM protocol_transactions WHERE tx_hash = LOWER($1));`,
		strings.TrimSpace(txHash),
	).Scan(&exists); err != nil {
		return false, fmt.Errorf("error checking whether transaction %s is claimed: %w", txHash, err)
	}
	return exists, nil
}
