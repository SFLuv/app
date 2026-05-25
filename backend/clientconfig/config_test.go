package clientconfig

import (
	"encoding/json"
	"strings"
	"testing"
)

const testConfigJSON = `{
  "community": {
    "name": "Test Community",
    "alias": "test.wallet",
    "profile": { "address": "0x0000000000000000000000000000000000000001", "chain_id": 80094 },
    "primary_token": { "address": "0x1111111111111111111111111111111111111111", "chain_id": 80094 },
    "primary_account_factory": { "address": "0x2222222222222222222222222222222222222222", "chain_id": 80094 }
  },
  "tokens": {
    "80094:0x1111111111111111111111111111111111111111": {
      "standard": "erc20",
      "name": "Test Token",
      "address": "0x1111111111111111111111111111111111111111",
      "symbol": "TEST",
      "decimals": 18,
      "chain_id": 80094
    }
  },
  "accounts": {
    "80094:0x2222222222222222222222222222222222222222": {
      "chain_id": 80094,
      "entrypoint_address": "0x3333333333333333333333333333333333333333",
      "paymaster_address": "0x4444444444444444444444444444444444444444",
      "account_factory_address": "0x2222222222222222222222222222222222222222",
      "paymaster_type": "cw-safe"
    }
  },
  "chains": {
    "80094": {
      "id": 80094,
      "node": { "url": "https://80094.engine.citizenwallet.xyz" }
    }
  },
  "extras": {
    "custom_value": "keep-me",
    "faucet_address": "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  },
  "version": 1
}`

func TestParseMergesEnvironmentExtrasIntoRawJSON(t *testing.T) {
	clearExtrasEnv(t)
	t.Setenv("NEXT_PUBLIC_HONEY_ADDRESS", "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	t.Setenv("HONEY_DECIMALS", "18")
	t.Setenv("NEXT_PUBLIC_BYUSD_ADDRESS", "0xcccccccccccccccccccccccccccccccccccccccc")
	t.Setenv("BYUSD_DECIMALS", "6")
	t.Setenv("ZAPPER_ADDRESS", "0xdddddddddddddddddddddddddddddddddddddddd")
	t.Setenv("FAUCET_ADDRESS", "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee")
	t.Setenv("BACKING_ASSETS", "0xffffffffffffffffffffffffffffffffffffffff,0x9999999999999999999999999999999999999999")

	cfg, err := parse([]byte(testConfigJSON), "test")
	if err != nil {
		t.Fatalf("parse() error = %v", err)
	}
	if cfg.Extras.HoneyTokenAddress != "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("HoneyTokenAddress = %q", cfg.Extras.HoneyTokenAddress)
	}
	if cfg.Extras.HoneyDecimals == nil || *cfg.Extras.HoneyDecimals != 18 {
		t.Fatalf("HoneyDecimals = %v", cfg.Extras.HoneyDecimals)
	}
	if cfg.Extras.ByusdTokenAddress != "0xcccccccccccccccccccccccccccccccccccccccc" {
		t.Fatalf("ByusdTokenAddress = %q", cfg.Extras.ByusdTokenAddress)
	}
	if cfg.Extras.ByusdDecimals == nil || *cfg.Extras.ByusdDecimals != 6 {
		t.Fatalf("ByusdDecimals = %v", cfg.Extras.ByusdDecimals)
	}
	if cfg.Extras.ZapperAddress != "0xdddddddddddddddddddddddddddddddddddddddd" {
		t.Fatalf("ZapperAddress = %q", cfg.Extras.ZapperAddress)
	}
	if cfg.Extras.FaucetAddress != "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee" {
		t.Fatalf("FaucetAddress = %q", cfg.Extras.FaucetAddress)
	}
	if len(cfg.Extras.BackingAssets) != 2 {
		t.Fatalf("BackingAssets = %#v", cfg.Extras.BackingAssets)
	}

	var response map[string]any
	if err := json.Unmarshal(cfg.RawJSON(), &response); err != nil {
		t.Fatalf("RawJSON unmarshal error = %v", err)
	}
	extras, ok := response["extras"].(map[string]any)
	if !ok {
		t.Fatalf("response extras missing: %#v", response["extras"])
	}
	if extras["custom_value"] != "keep-me" {
		t.Fatalf("custom extras field was not preserved: %#v", extras)
	}
	if extras["faucet_address"] != "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee" {
		t.Fatalf("faucet_address was not overridden by env: %#v", extras["faucet_address"])
	}
}

func TestParseOmitsTokenExtrasWhenCitizenWalletTokensExist(t *testing.T) {
	clearExtrasEnv(t)
	t.Setenv("HONEY_ADDRESS", "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	t.Setenv("HONEY_DECIMALS", "18")
	t.Setenv("BYUSD_ADDRESS", "0xcccccccccccccccccccccccccccccccccccccccc")
	t.Setenv("BYUSD_DECIMALS", "6")

	body := strings.Replace(testConfigJSON, `    }
  },
  "accounts":`, `    },
    "80094:0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb": {
      "standard": "erc20",
      "name": "Honey",
      "address": "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
      "symbol": "HONEY",
      "decimals": 18,
      "chain_id": 80094
    },
    "80094:0xcccccccccccccccccccccccccccccccccccccccc": {
      "standard": "erc20",
      "name": "BYUSD",
      "address": "0xcccccccccccccccccccccccccccccccccccccccc",
      "symbol": "BYUSD",
      "decimals": 6,
      "chain_id": 80094
    }
  },
  "accounts":`, 1)
	body = strings.Replace(body, `  "extras": {
    "custom_value": "keep-me",
    "faucet_address": "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  },`, `  "extras": {
    "honey_token_address": "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "byusd_token_address": "0x9999999999999999999999999999999999999999"
  },`, 1)

	cfg, err := parse([]byte(body), "test")
	if err != nil {
		t.Fatalf("parse() error = %v", err)
	}
	if cfg.Extras.HoneyTokenAddress != "" || cfg.Extras.HoneyDecimals != nil {
		t.Fatalf("honey extras should be omitted when HONEY token exists: %#v", cfg.Extras)
	}
	if cfg.Extras.ByusdTokenAddress != "" || cfg.Extras.ByusdDecimals != nil {
		t.Fatalf("byusd extras should be omitted when BYUSD token exists: %#v", cfg.Extras)
	}

	var response struct {
		Extras map[string]any `json:"extras"`
	}
	if err := json.Unmarshal(cfg.RawJSON(), &response); err != nil {
		t.Fatalf("RawJSON unmarshal error = %v", err)
	}
	if _, ok := response.Extras["honey_token_address"]; ok {
		t.Fatalf("honey_token_address should not be in response extras: %#v", response.Extras)
	}
	if _, ok := response.Extras["byusd_token_address"]; ok {
		t.Fatalf("byusd_token_address should not be in response extras: %#v", response.Extras)
	}
}

func clearExtrasEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"HONEY_TOKEN_ADDRESS",
		"HONEY_ADDRESS",
		"HONEY_TOKEN",
		"HONEY_DECIMALS",
		"NEXT_PUBLIC_HONEY_TOKEN_ADDRESS",
		"NEXT_PUBLIC_HONEY_ADDRESS",
		"NEXT_PUBLIC_HONEY_TOKEN",
		"NEXT_PUBLIC_HONEY_DECIMALS",
		"BYUSD_TOKEN_ADDRESS",
		"BYUSD_ADDRESS",
		"BYUSD_TOKEN",
		"BYUSD_DECIMALS",
		"NEXT_PUBLIC_BYUSD_TOKEN_ADDRESS",
		"NEXT_PUBLIC_BYUSD_ADDRESS",
		"NEXT_PUBLIC_BYUSD_TOKEN",
		"NEXT_PUBLIC_BYUSD_DECIMALS",
		"ZAPPER_CONTRACT_ADDRESS",
		"ZAPPER_ADDRESS",
		"NEXT_PUBLIC_ZAPPER_CONTRACT_ADDRESS",
		"NEXT_PUBLIC_ZAPPER_ADDRESS",
		"FAUCET_ADDRESS",
		"NEXT_PUBLIC_FAUCET_ADDRESS",
		"BACKING_ASSETS",
		"NEXT_PUBLIC_BACKING_ASSETS",
	} {
		t.Setenv(key, "")
	}
}
