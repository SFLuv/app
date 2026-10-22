// Command protocoldryrun asks the paymaster what it is willing to sponsor,
// before any of it is switched on in production.
//
// Nothing about the cutover has ever been submitted to the engine. The whole
// design assumes the Citizen Wallet paymaster will sponsor five different kinds
// of operation, and it might not: a paymaster may restrict sponsorship to
// certain targets or selectors, or require the account to hold the community
// token, and any one of these could be refused while the others work. Finding
// that out by enabling the cutover in production is the expensive way.
//
// So this builds each kind of operation for a throwaway account and asks the
// paymaster to sponsor it. Sponsorship is decided before execution, so the
// answer comes back without anything being signed, submitted or spent — and
// from an empty wallet, which is the state a real Safe is in before its first
// use.
//
//	go run ./cmd/protocoldryrun
//
// The operations tested, matching the ones the cutover needs:
//
//	deploy        a Safe through the account factory (initCode)
//	transfer      an ERC-20 transfer of the community token
//	swap-owner    swapOwner on the Safe itself, which is key rotation
//	grant-role    grantRole on the token, which is the redeemer handover
//	revoke-role   revokeRole on the token
//	native        a transfer carrying CELO value
//
// It also sponsors twice in a row and reports both nonces, which answers
// whether operations from one account can be pipelined or have to be
// serialised.
package main

import (
	"context"
	"crypto/ecdsa"
	"flag"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/bootstrap"
	"github.com/SFLuv/app/backend/clientconfig"
	"github.com/SFLuv/app/backend/protocol"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func main() {
	index := flag.Int("index", 0, "factory salt for the throwaway Safe")
	flag.Parse()

	bootstrap.LoadEnv()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	config, err := clientconfig.Load(ctx)
	if err != nil {
		fail("client config could not be loaded: %s", err)
	}
	token, err := config.PrimaryToken()
	if err != nil {
		fail("primary token is unreadable: %s", err)
	}
	account, err := config.PrimaryAccount()
	if err != nil {
		fail("primary account config is unreadable: %s", err)
	}

	client, err := protocol.NewClient(protocol.Config{
		EngineBaseURL: config.PrimaryRPCURL(),
		ReadRPCURL:    config.ReadRPCURL(),
		EntryPoint:    account.EntryPointAddress,
		Factory:       account.AccountFactoryAddress,
		Paymaster:     account.PaymasterAddress,
		PaymasterType: account.PaymasterType,
	})
	if err != nil {
		fail("protocol client could not be built: %s", err)
	}

	// A key that exists for the length of this run and is never stored. Its
	// Safe is empty and undeployed, which is exactly the state being tested.
	key, owner, err := protocol.GenerateSigner()
	if err != nil {
		fail("could not mint a throwaway key: %s", err)
	}
	_ = key

	safe, err := client.DeriveAddress(ctx, owner, big.NewInt(int64(*index)))
	if err != nil {
		fail("could not derive the throwaway Safe: %s", err)
	}
	exists, err := client.AccountExists(ctx, safe)
	if err != nil {
		fail("could not ask whether the throwaway Safe exists: %s", err)
	}

	tokenAddress := common.HexToAddress(token.Address)
	fmt.Printf("chain          %d\n", config.ActiveChainID())
	fmt.Printf("engine         %s\n", config.PrimaryRPCURL())
	fmt.Printf("paymaster      %s (%s)\n", account.PaymasterAddress, account.PaymasterType)
	fmt.Printf("entry point    %s\n", account.EntryPointAddress)
	fmt.Printf("token          %s\n", tokenAddress.Hex())
	fmt.Printf("throwaway EOA  %s\n", owner.Hex())
	fmt.Printf("throwaway Safe %s (deployed: %v)\n\n", safe.Hex(), exists)

	// Every operation carries initCode while the Safe is undeployed, which is
	// how the real first operation of any account looks.
	initCode := []byte{}
	if !exists {
		if initCode, err = client.InitCode(owner, big.NewInt(int64(*index))); err != nil {
			fail("could not build the deployment init code: %s", err)
		}
	}

	transfer, err := protocol.TransferCallData(owner, big.NewInt(1))
	orDie(err)
	transferCall, err := protocol.SafeModuleCallData(tokenAddress, big.NewInt(0), transfer)
	orDie(err)

	swap, err := protocol.SwapOwnerCallData([]common.Address{owner}, owner, crypto.PubkeyToAddress(mustKey().PublicKey))
	orDie(err)
	swapCall, err := protocol.SafeModuleCallData(safe, big.NewInt(0), swap)
	orDie(err)

	var role [32]byte
	copy(role[:], crypto.Keccak256([]byte("REDEEMER_ROLE")))
	grant, err := protocol.GrantRoleCallData(role, owner)
	orDie(err)
	grantCall, err := protocol.SafeModuleCallData(tokenAddress, big.NewInt(0), grant)
	orDie(err)
	revoke, err := protocol.RevokeRoleCallData(role, owner)
	orDie(err)
	revokeCall, err := protocol.SafeModuleCallData(tokenAddress, big.NewInt(0), revoke)
	orDie(err)

	// A value-carrying call, which is what a CELO sweep out of a Safe would be.
	nativeCall, err := protocol.SafeModuleCallData(owner, big.NewInt(1), nil)
	orDie(err)

	// Two transfers in one operation, through MultiSend. This is the batching
	// the faucet relies on under load, and it is a delegatecall — the one
	// operation shape a paymaster is most likely to treat differently.
	batchFirst, err := protocol.TransferCallData(owner, big.NewInt(1))
	orDie(err)
	batchCall, err := protocol.BatchCallData([]protocol.BatchedCall{
		{To: tokenAddress, Value: big.NewInt(0), Data: batchFirst},
		{To: tokenAddress, Value: big.NewInt(0), Data: batchFirst},
	})
	orDie(err)

	cases := []struct {
		name     string
		callData []byte
	}{
		{"deploy (inert self-call + initCode)", mustInert(safe)},
		{"transfer (ERC-20)", transferCall},
		{"batch of 2 transfers (MultiSend, delegatecall)", batchCall},
		{"swap-owner (key rotation)", swapCall},
		{"grant-role (redeemer handover)", grantCall},
		{"revoke-role", revokeCall},
		{"native value transfer", nativeCall},
	}

	failures := 0
	for _, testCase := range cases {
		op := protocol.NewUserOperation(safe)
		op.CallData = testCase.callData
		op.InitCode = initCode

		sponsored, err := client.Sponsor(ctx, op)
		if err != nil {
			failures++
			fmt.Printf("REFUSED  %-46s %s\n", testCase.name, oneLine(err.Error()))
			continue
		}
		fmt.Printf("sponsored %-46s nonce %s, callGas %s, verificationGas %s\n",
			testCase.name,
			sponsored.Nonce, sponsored.CallGasLimit, sponsored.VerificationGasLimit)
	}

	// Does a second sponsorship advance the nonce? If it does not, two
	// operations in flight at once would collide and they must stay
	// serialised; if it does, the faucet could pipeline them.
	fmt.Println()
	first := protocol.NewUserOperation(safe)
	first.CallData = transferCall
	first.InitCode = initCode
	second := protocol.NewUserOperation(safe)
	second.CallData = transferCall
	second.InitCode = initCode

	sponsoredFirst, errFirst := client.Sponsor(ctx, first)
	sponsoredSecond, errSecond := client.Sponsor(ctx, second)
	switch {
	case errFirst != nil || errSecond != nil:
		fmt.Println("nonce sequencing: could not be tested, sponsorship was refused")
	case sponsoredFirst.Nonce.Cmp(sponsoredSecond.Nonce) == 0:
		fmt.Printf("nonce sequencing: BOTH got nonce %s — operations must stay serialised\n", sponsoredFirst.Nonce)
	default:
		fmt.Printf("nonce sequencing: %s then %s — the engine advances it, so pipelining is possible\n",
			sponsoredFirst.Nonce, sponsoredSecond.Nonce)
	}

	fmt.Println()
	if failures > 0 {
		fmt.Printf("%d of %d operation types were refused. Do not enable the cutover until these are understood:\n", failures, len(cases))
		fmt.Println("a refusal may be the paymaster's policy, or it may be that an empty Safe cannot pay its")
		fmt.Println("sponsorship fee — the error text above is the only thing that distinguishes them.")
		os.Exit(1)
	}
	fmt.Printf("all %d operation types were sponsored for an empty, undeployed Safe.\n", len(cases))
	fmt.Println("nothing was signed, submitted or spent by this run.")
}

// mustInert is the deploying operation's call: the Safe calls itself with no
// data, so the operation's only effect is the deployment in its init code.
func mustInert(safe common.Address) []byte {
	data, err := protocol.SafeModuleCallData(safe, big.NewInt(0), nil)
	orDie(err)
	return data
}

func mustKey() *ecdsa.PrivateKey {
	key, _, err := protocol.GenerateSigner()
	orDie(err)
	return key
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func orDie(err error) {
	if err != nil {
		fail("%s", err)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "dry run: "+format+"\n", args...)
	os.Exit(1)
}
