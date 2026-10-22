package handlers

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/protocol"
	"github.com/SFLuv/app/backend/structs"
	"github.com/ethereum/go-ethereum/common"
)

// Key rotation.
//
// One sponsored operation per account: the Safe calls swapOwner on itself,
// replacing the signer in a single transaction. Not addOwnerWithThreshold
// followed by removeOwner, which is the shape the request described — those are
// two operations, and between them the Safe trusts both keys, including the one
// being rotated away from. When the reason for rotating is that the old key may
// be in someone else's hands, a window where it still signs is the one thing
// worth engineering away.
//
// The Safe's address does not change. That is the entire point of having moved
// to Safes: a rotation is invisible to every address check, every balance read
// and every past payout, because the identity that holds the money is not the
// key that signs for it.

const rotationConfirmTimeout = 90 * time.Second

// Rotate replaces one account's signing key.
func (p *ProtocolService) Rotate(ctx context.Context, accountKey, actor string) (*structs.ProtocolRotateResponse, error) {
	if err := p.Available(); err != nil {
		return nil, err
	}

	// One rotation at a time across the process. Two concurrent rotations of
	// the same account would each read the same owner set and build a swap from
	// the same predecessor, and the second would revert against a Safe that had
	// already moved on.
	p.mu.Lock()
	defer p.mu.Unlock()

	account, err := p.db.GetProtocolAccount(ctx, accountKey)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, fmt.Errorf("protocol account %s is not registered", accountKey)
	}
	if !common.IsHexAddress(account.SafeAddress) || account.DeployedAt == nil {
		return nil, fmt.Errorf("protocol account %s has no deployed Safe to rotate", accountKey)
	}
	if account.Signer == nil {
		return nil, fmt.Errorf("protocol account %s has no active signer to rotate away from", accountKey)
	}

	safe := common.HexToAddress(account.SafeAddress)
	oldEOA := common.HexToAddress(account.Signer.EOAAddress)

	owners, err := p.client.Owners(ctx, safe)
	if err != nil {
		return nil, err
	}

	newKey, newEOA, err := protocol.GenerateSigner()
	if err != nil {
		return nil, err
	}
	sealed, err := protocol.SealSigner(p.master, accountKey, newKey)
	if err != nil {
		return nil, err
	}
	// Recorded before it is used. A key that signs for a Safe and is not in the
	// database is a key nobody can ever decrypt again — the account would be
	// live, owned, and unspendable.
	newID, err := p.db.InsertProtocolSigner(ctx, accountKey, strings.ToLower(newEOA.Hex()), sealed, actor)
	if err != nil {
		return nil, err
	}

	abandon := func(reason string) {
		if err := p.db.AbandonProtocolSigner(ctx, newID, reason); err != nil {
			p.logf("protocol rotation %s: could not abandon unused signer %d: %s", accountKey, newID, err)
		}
	}

	swapData, err := protocol.SwapOwnerCallData(owners, oldEOA, newEOA)
	if err != nil {
		abandon("swap could not be encoded")
		return nil, err
	}

	// to is the Safe itself: swapOwner is a self-call, so the module executes it
	// with the Safe as msg.sender.
	opHash, err := p.Execute(ctx, accountKey, safe, big.NewInt(0), swapData)
	if err != nil {
		abandon("rotation operation was not accepted")
		return nil, fmt.Errorf("error submitting the rotation for %s: %w", accountKey, err)
	}

	if err := p.awaitOwnerChange(ctx, safe, oldEOA, newEOA); err != nil {
		// Deliberately not abandoned. The operation was accepted and may still
		// land, and if it does the new key is the only one that can sign for
		// this Safe — discarding the row would lose it. It stays pending, and
		// the next call reuses it rather than minting another.
		return nil, fmt.Errorf(
			"rotation %s for %s was submitted but the Safe's owners have not changed yet: %w",
			opHash, accountKey, err,
		)
	}

	if err := p.db.PromoteProtocolSigner(ctx, accountKey, newID, opHash, "rotated"); err != nil {
		return nil, fmt.Errorf(
			"the Safe %s now answers to %s but the database could not be updated: %w",
			safe.Hex(), newEOA.Hex(), err,
		)
	}

	p.logf("protocol rotation %s: Safe %s moved from signer %s to %s (%s)",
		accountKey, safe.Hex(), oldEOA.Hex(), newEOA.Hex(), opHash)

	return &structs.ProtocolRotateResponse{
		AccountKey:    accountKey,
		SafeAddress:   safe.Hex(),
		PreviousEOA:   oldEOA.Hex(),
		NewEOA:        newEOA.Hex(),
		OperationHash: opHash,
		Message:       "The Safe's address is unchanged. The previous key is no longer an owner.",
	}, nil
}

// awaitOwnerChange waits until the Safe itself confirms the swap.
//
// Both halves are checked. A rotation that added the new owner without removing
// the old one would leave the compromised key signing, which reads as success
// from the operation's point of view and is the failure that matters here.
func (p *ProtocolService) awaitOwnerChange(ctx context.Context, safe, oldEOA, newEOA common.Address) error {
	deadline := time.Now().Add(rotationConfirmTimeout)
	for {
		owners, err := p.client.Owners(ctx, safe)
		if err == nil {
			hasNew, hasOld := false, false
			for _, owner := range owners {
				if owner == newEOA {
					hasNew = true
				}
				if owner == oldEOA {
					hasOld = true
				}
			}
			if hasNew && !hasOld {
				return nil
			}
			if hasNew && hasOld {
				return fmt.Errorf("the new key was added but %s is still an owner", oldEOA.Hex())
			}
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("the Safe's owners were unchanged after %s", rotationConfirmTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

// RotateAll rotates every account that can be rotated, and reports each one.
//
// Sequential, and it does not stop at the first failure. The accounts are
// independent, and when the reason for rotating is a suspected leak, rotating
// three of four is better than rotating none — so each result is reported
// separately and the caller is told exactly which keys are new.
func (p *ProtocolService) RotateAll(ctx context.Context, actor string) *structs.ProtocolRotateAllResponse {
	response := &structs.ProtocolRotateAllResponse{
		Rotated: []structs.ProtocolRotateResponse{},
		Failed:  []structs.ProtocolRotateFailure{},
	}
	if err := p.Available(); err != nil {
		response.Failed = append(response.Failed, structs.ProtocolRotateFailure{AccountKey: "", Error: err.Error()})
		return response
	}

	accounts, err := p.Accounts(ctx)
	if err != nil {
		response.Failed = append(response.Failed, structs.ProtocolRotateFailure{AccountKey: "", Error: err.Error()})
		return response
	}

	for _, account := range accounts {
		if account.DeployedAt == nil || account.Signer == nil {
			response.Failed = append(response.Failed, structs.ProtocolRotateFailure{
				AccountKey: account.Key,
				Error:      "no deployed Safe with an active signer",
			})
			continue
		}
		rotated, err := p.Rotate(ctx, account.Key, actor)
		if err != nil {
			response.Failed = append(response.Failed, structs.ProtocolRotateFailure{
				AccountKey: account.Key,
				Error:      err.Error(),
			})
			continue
		}
		response.Rotated = append(response.Rotated, *rotated)
	}
	return response
}
