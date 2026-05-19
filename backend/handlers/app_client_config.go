package handlers

import (
	"encoding/json"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
)

const (
	defaultBerachainID         = 80094
	defaultCeloID              = 42220
	defaultBerachainToken      = "0x881cad4f885c6701d8481c0ed347f6d35444ea7e"
	defaultCeloUSDC            = "0xcebA9300f2b948710d2653dD7B07f33A8B32118C"
	defaultAccountFactory      = "0x7cC54D54bBFc65d1f0af7ACee5e4042654AF8185"
	defaultBerachainEntryPoint = "0x7079253c0358eF9Fd87E16488299Ef6e06F403B6"
	defaultBerachainPaymaster  = "0x9A5be02B65f9Aa00060cB8c951dAFaBAB9B860cd"
)

func envString(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return strings.EqualFold(value, "true") || value == "1" || strings.EqualFold(value, "yes")
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func activeChainID() int {
	return envInt("CHAIN_ID", defaultBerachainID)
}

func tokenDecimalPlaces() int {
	raw := strings.TrimSpace(os.Getenv("TOKEN_DECIMALS"))
	if raw == "" {
		return 18
	}

	if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 36 {
		return parsed
	}

	multiplier, ok := new(big.Int).SetString(raw, 10)
	if !ok || multiplier.Sign() <= 0 {
		return 18
	}

	ten := big.NewInt(10)
	zero := big.NewInt(0)
	one := big.NewInt(1)
	places := 0
	for multiplier.Cmp(one) > 0 && places <= 36 {
		quotient := new(big.Int)
		remainder := new(big.Int)
		quotient.QuoRem(multiplier, ten, remainder)
		if remainder.Cmp(zero) != 0 {
			return 18
		}
		multiplier = quotient
		places++
	}
	if multiplier.Cmp(one) != 0 {
		return 18
	}
	return places
}

func configVersion() string {
	return envString("CLIENT_CONFIG_VERSION", time.Now().UTC().Format("2006-01-02"))
}

func currentEnvironment() string {
	if envBool("IN_PRODUCTION", false) {
		return "production"
	}
	return envString("APP_ENVIRONMENT", "development")
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (a *AppService) GetClientConfig(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	chainID := activeChainID()
	tokenDecimals := tokenDecimalPlaces()

	chainName := envString("CHAIN_NAME", "Berachain")
	nativeName := envString("NATIVE_CURRENCY_NAME", "BERA")
	nativeSymbol := envString("NATIVE_CURRENCY_SYMBOL", "BERA")
	explorerName := envString("EXPLORER_NAME", "Berascan")
	explorerURL := envString("EXPLORER_URL", "https://berascan.com")
	engineRPC := envString("ENGINE_RPC_URL", "https://80094.engine.citizenwallet.xyz")
	engineWS := envString("ENGINE_WS_URL", "wss://80094.engine.citizenwallet.xyz")

	response := structs.ClientConfigResponse{
		SchemaVersion:  1,
		ConfigVersion:  configVersion(),
		Environment:    currentEnvironment(),
		ActiveChainID:  chainID,
		LegacyChainIDs: []int{defaultBerachainID},
		Community: structs.ClientCommunityConfig{
			Name:           envString("CLIENT_COMMUNITY_NAME", "SFLUV Community"),
			Alias:          envString("CLIENT_COMMUNITY_ALIAS", "wallet.berachain.sfluv.org"),
			CustomDomain:   envString("CLIENT_COMMUNITY_CUSTOM_DOMAIN", "wallet.sfluv.org"),
			Logo:           envString("CLIENT_COMMUNITY_LOGO", "https://assets.citizenwallet.xyz/wallet-config/_images/sfluv.svg"),
			ProfileAddress: envString("CLIENT_COMMUNITY_PROFILE_ADDRESS", "0x05e2Fb34b4548990F96B3ba422eA3EF49D5dAa99"),
		},
		Chains: map[string]structs.ClientChainConfig{
			strconv.Itoa(chainID): {
				ID:   chainID,
				Name: chainName,
				NativeCurrency: structs.ClientNativeCurrency{
					Name:     nativeName,
					Symbol:   nativeSymbol,
					Decimals: envInt("NATIVE_CURRENCY_DECIMALS", 18),
				},
				RPCURL:       envString("RPC_URL", "https://rpc.berachain.com"),
				EngineRPCURL: engineRPC,
				EngineWSURL:  engineWS,
				Explorer:     structs.ClientExplorerConfig{Name: explorerName, URL: explorerURL},
			},
		},
		Tokens: map[string]structs.ClientTokenConfig{
			"primary": {
				Standard: "erc20",
				Name:     envString("TOKEN_NAME", "SFLUV"),
				Symbol:   envString("TOKEN_SYMBOL", "SFLUV"),
				Address:  envString("TOKEN_ID", defaultBerachainToken),
				ChainID:  chainID,
				Decimals: tokenDecimals,
			},
			"celo_usdc": {
				Standard: "erc20",
				Name:     "USDC",
				Symbol:   "USDC",
				Address:  envString("CELO_USDC_ADDRESS", defaultCeloUSDC),
				ChainID:  defaultCeloID,
				Decimals: 6,
			},
		},
		Accounts: map[string]structs.ClientAccountConfig{
			"primary": {
				ChainID:               chainID,
				EntryPointAddress:     envString("ENTRYPOINT_ADDRESS", defaultBerachainEntryPoint),
				AccountFactoryAddress: envString("ACCOUNT_FACTORY_ADDRESS", defaultAccountFactory),
				PaymasterAddress:      envString("PAYMASTER_ADDRESS", defaultBerachainPaymaster),
				PaymasterType:         envString("PAYMASTER_TYPE", "cw-safe"),
			},
		},
		URLs: structs.ClientURLConfig{
			AppOrigin:                   envString("APP_BASE_URL", "https://app.sfluv.org"),
			Backend:                     envString("PUBLIC_BACKEND_URL", "https://api.sfluv.org"),
			CitizenWalletConfigLocation: envString("CITIZEN_WALLET_CONFIG_URL", "https://config.internal.citizenwallet.xyz/v4/wallet.sfluv.org.json"),
			IPFS:                        envString("IPFS_URL", "https://ipfs.internal.citizenwallet.xyz"),
		},
		Features: structs.ClientFeatureConfig{
			MigrationBanner:         envBool("CLIENT_MIGRATION_BANNER", false),
			SendsEnabled:            envBool("FEATURE_SENDS_ENABLED", true),
			RedemptionsEnabled:      envBool("FEATURE_REDEMPTIONS_ENABLED", true),
			WorkflowPayoutsEnabled:  envBool("FEATURE_WORKFLOW_PAYOUTS_ENABLED", true),
			MerchantPaymentsEnabled: envBool("FEATURE_MERCHANT_PAYMENTS_ENABLED", true),
		},
		Migration: structs.ClientMigrationConfig{
			State:            envString("MIGRATION_STATE", "pre_cutover"),
			Message:          envString("MIGRATION_MESSAGE", ""),
			CutoverStartedAt: nil,
		},
		Source: structs.ClientConfigSource{
			Provider:     "backend",
			FallbackUsed: false,
			FetchedAt:    now.Format(time.RFC3339),
		},
	}

	writeJSON(w, http.StatusOK, response)
}

func (a *AppService) GetClientVersion(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	platform := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("platform")))
	if platform == "" {
		platform = "unknown"
	}

	build := envInt("CLIENT_DEFAULT_BUILD", 0)
	if requestedBuild := strings.TrimSpace(r.URL.Query().Get("build")); requestedBuild != "" {
		if parsed, err := strconv.Atoi(requestedBuild); err == nil {
			build = parsed
		}
	}
	version := strings.TrimSpace(r.URL.Query().Get("version"))
	if version == "" {
		version = envString("CLIENT_DEFAULT_VERSION", "0.0.0")
	}

	minimum := structs.ClientVersionInfo{
		Version: envString("CLIENT_MIN_VERSION", "1.0.0"),
		Build:   envInt("CLIENT_MIN_BUILD", 1),
	}
	recommended := structs.ClientVersionInfo{
		Version: envString("CLIENT_RECOMMENDED_VERSION", minimum.Version),
		Build:   envInt("CLIENT_RECOMMENDED_BUILD", minimum.Build),
	}
	current := structs.ClientVersionInfo{
		Version: envString("CLIENT_CURRENT_VERSION", recommended.Version),
		Build:   envInt("CLIENT_CURRENT_BUILD", recommended.Build),
	}

	status := "ok"
	forceUpdate := false
	maintenance := envBool("CLIENT_MAINTENANCE", false)
	message := envString("CLIENT_VERSION_MESSAGE", "")

	if maintenance {
		status = "maintenance"
		message = envString("CLIENT_MAINTENANCE_MESSAGE", "SFLUV is temporarily unavailable while maintenance is in progress.")
	} else if platform != "ios" && platform != "android" && platform != "web" {
		status = "unsupported_platform"
		forceUpdate = true
		message = envString("CLIENT_UNSUPPORTED_PLATFORM_MESSAGE", "This app version is not supported.")
	} else if build < minimum.Build {
		status = "update_required"
		forceUpdate = true
		message = envString("CLIENT_UPDATE_REQUIRED_MESSAGE", "An SFLUV Wallet update is required.")
	} else if build < recommended.Build {
		status = "update_recommended"
		message = envString("CLIENT_UPDATE_RECOMMENDED_MESSAGE", "A newer SFLUV Wallet update is available.")
	}

	response := structs.ClientVersionResponse{
		SchemaVersion: 1,
		ServerTime:    now.Format(time.RFC3339),
		ConfigVersion: configVersion(),
		Platform:      platform,
		Status:        status,
		Minimum:       minimum,
		Recommended:   recommended,
		Current:       current,
		ForceUpdate:   forceUpdate,
		Maintenance:   maintenance,
		UpdateURL:     envString("CLIENT_UPDATE_URL_"+strings.ToUpper(platform), envString("CLIENT_UPDATE_URL", "")),
		Message:       message,
		Features: structs.ClientVersionFeatures{
			DynamicConfigRequired: envBool("CLIENT_DYNAMIC_CONFIG_REQUIRED", true),
			CeloRequired:          envBool("CLIENT_CELO_REQUIRED", activeChainID() == defaultCeloID),
		},
	}

	_ = version
	writeJSON(w, http.StatusOK, response)
}
