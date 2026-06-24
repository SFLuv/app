package clientconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// defaultCommunityAPIURL is the Citizen Wallet communities API base. The
	// whole community config is fetched from {base}/{alias}.
	defaultCommunityAPIURL = "https://my.citizenwallet.xyz/api/communities"
	defaultFallbackFile    = "community-config.json"
)

type AddressRef struct {
	Address string `json:"address"`
	ChainID int    `json:"chain_id"`
}

type Community struct {
	Name                  string     `json:"name"`
	Description           string     `json:"description,omitempty"`
	URL                   string     `json:"url,omitempty"`
	Alias                 string     `json:"alias"`
	CustomDomain          string     `json:"custom_domain,omitempty"`
	Logo                  string     `json:"logo,omitempty"`
	Profile               AddressRef `json:"profile"`
	PrimaryToken          AddressRef `json:"primary_token"`
	PrimaryAccountFactory AddressRef `json:"primary_account_factory"`
}

type Token struct {
	Standard string `json:"standard"`
	Name     string `json:"name"`
	Address  string `json:"address"`
	Symbol   string `json:"symbol"`
	Decimals int    `json:"decimals"`
	ChainID  int    `json:"chain_id"`
}

type Account struct {
	ChainID               int    `json:"chain_id"`
	EntryPointAddress     string `json:"entrypoint_address"`
	PaymasterAddress      string `json:"paymaster_address"`
	AccountFactoryAddress string `json:"account_factory_address"`
	PaymasterType         string `json:"paymaster_type"`
}

type ChainNode struct {
	URL   string `json:"url"`
	WSURL string `json:"ws_url,omitempty"`
}

type Chain struct {
	ID   int       `json:"id"`
	Node ChainNode `json:"node"`
}

type Extras struct {
	HoneyTokenAddress string   `json:"honey_token_address,omitempty"`
	HoneyDecimals     *int     `json:"honey_decimals,omitempty"`
	ByusdTokenAddress string   `json:"byusd_token_address,omitempty"`
	ByusdDecimals     *int     `json:"byusd_decimals,omitempty"`
	ZapperAddress     string   `json:"zapper_address,omitempty"`
	FaucetAddress     string   `json:"faucet_address,omitempty"`
	BackingAssets     []string `json:"backing_assets,omitempty"`
	// ReadRPCURL is a full JSON-RPC node URL for general reads (eth_getCode,
	// eth_getBalance, eth_getStorageAt, ...). The Citizen Wallet engine at
	// chains[].node.url is a curated RPC that 404s those methods, so clients
	// must use this for reads and reserve the engine for the AA bundler. Sourced
	// from the backend RPC_URL env, taking precedence over the engine URL.
	ReadRPCURL string `json:"rpc_url,omitempty"`
}

type Config struct {
	Community      Community          `json:"community"`
	Tokens         map[string]Token   `json:"tokens"`
	Accounts       map[string]Account `json:"accounts"`
	Chains         map[string]Chain   `json:"chains"`
	Extras         Extras             `json:"extras,omitempty"`
	ConfigLocation string             `json:"config_location,omitempty"`
	Version        int                `json:"version"`

	raw    []byte
	source string
}

func Load(ctx context.Context) (*Config, error) {
	if clientConfigLocalOnly() {
		cfg, err := loadFallback()
		if err != nil {
			return nil, fmt.Errorf("unable to load local client config: %w", err)
		}
		return cfg, nil
	}

	if cfg, err := loadRemote(ctx); err == nil {
		return cfg, nil
	} else if fallback, fallbackErr := loadFallback(); fallbackErr == nil {
		return fallback, nil
	} else {
		return nil, fmt.Errorf("unable to load client config from Citizen Wallet (%v) or fallback file (%v)", err, fallbackErr)
	}
}

func (c *Config) RawJSON() []byte {
	if c == nil {
		return nil
	}
	return append([]byte(nil), c.raw...)
}

func (c *Config) Source() string {
	if c == nil {
		return ""
	}
	return c.source
}

func (c *Config) ActiveChainID() int {
	if c == nil {
		return 0
	}
	return c.Community.PrimaryToken.ChainID
}

func (c *Config) PrimaryToken() (Token, error) {
	if c == nil {
		return Token{}, fmt.Errorf("client config is nil")
	}
	return findToken(c.Tokens, c.Community.PrimaryToken)
}

func (c *Config) PrimaryAccount() (Account, error) {
	if c == nil {
		return Account{}, fmt.Errorf("client config is nil")
	}
	return findAccount(c.Accounts, c.Community.PrimaryAccountFactory)
}

func (c *Config) PrimaryRPCURL() string {
	if c == nil {
		return ""
	}
	chain := c.Chains[strconv.Itoa(c.ActiveChainID())]
	return strings.TrimSpace(chain.Node.URL)
}

// ReadRPCURL is the JSON-RPC endpoint to use for general read calls
// (eth_call, eth_chainId, eth_getCode, ...). The Citizen Wallet engine at
// PrimaryRPCURL() is a curated RPC reserved for the AA bundler and rejects
// those methods, so a full node RPC (sourced from the RPC_URL env into
// extras.rpc_url) takes precedence when present, falling back to the engine
// URL only if no read RPC is configured.
func (c *Config) ReadRPCURL() string {
	if c == nil {
		return ""
	}
	if url := strings.TrimSpace(c.Extras.ReadRPCURL); url != "" {
		return url
	}
	return c.PrimaryRPCURL()
}

func loadRemote(ctx context.Context) (*Config, error) {
	// An explicit single-community config URL wins when set (a raw config object).
	explicit, err := explicitConfigURL()
	if err != nil {
		return nil, err
	}
	if explicit != "" {
		body, err := fetchRemote(ctx, explicit)
		if err != nil {
			return nil, err
		}
		return parse(body, "citizenwallet:"+explicit)
	}

	// Otherwise fetch the whole community config from the Citizen Wallet
	// communities API by alias. The endpoint returns an envelope whose "json"
	// field holds the full config (community, tokens, accounts, chains, ...).
	alias, err := communityAlias()
	if err != nil {
		return nil, err
	}
	communityURL, err := communityConfigURL(alias)
	if err != nil {
		return nil, err
	}
	body, err := fetchRemote(ctx, communityURL)
	if err != nil {
		return nil, err
	}
	inner, err := extractCommunityConfig(body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", communityURL, err)
	}
	return parse(inner, "citizenwallet:"+communityURL)
}

func fetchRemote(ctx context.Context, remoteURL string) ([]byte, error) {
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, remoteURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s returned %d", remoteURL, res.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	return body, nil
}

func loadFallback() (*Config, error) {
	path := strings.TrimSpace(os.Getenv("CLIENT_CONFIG_FALLBACK_PATH"))
	if path == "" {
		path = strings.TrimSpace(os.Getenv("CITIZEN_WALLET_CONFIG_FALLBACK_PATH"))
	}
	if path == "" {
		path = defaultFallbackFile
	}

	var lastErr error
	for _, candidate := range fallbackCandidates(path) {
		body, err := os.ReadFile(candidate)
		if err != nil {
			lastErr = err
			continue
		}
		return parse(body, "file:"+candidate)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no fallback candidates for %s", path)
	}
	return nil, lastErr
}

func fallbackCandidates(path string) []string {
	if filepath.IsAbs(path) {
		return []string{path}
	}

	candidates := []string{path}
	if filepath.Dir(path) == "." {
		candidates = append(candidates, filepath.Join("backend", path))
	}
	return candidates
}

func explicitConfigURL() (string, error) {
	configured := strings.TrimSpace(os.Getenv("CITIZEN_WALLET_CONFIG_URL"))
	if configured == "" {
		return "", nil
	}
	if _, err := url.ParseRequestURI(configured); err != nil {
		return "", fmt.Errorf("invalid CITIZEN_WALLET_CONFIG_URL %q: %w", configured, err)
	}
	return configured, nil
}

func communityAlias() (string, error) {
	alias := strings.TrimSpace(os.Getenv("CITIZEN_WALLET_COMMUNITY_ALIAS"))
	if alias == "" {
		alias = strings.TrimSpace(os.Getenv("CLIENT_COMMUNITY_ALIAS"))
	}
	if alias == "" {
		return "", fmt.Errorf("CITIZEN_WALLET_COMMUNITY_ALIAS is not set")
	}
	return alias, nil
}

// communityConfigURL builds the Citizen Wallet communities API URL for an
// alias: {base}/{alias}. The base defaults to the public communities API and is
// overridable with CITIZEN_WALLET_COMMUNITY_API_URL.
func communityConfigURL(alias string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("CITIZEN_WALLET_COMMUNITY_API_URL")), "/")
	if base == "" {
		base = defaultCommunityAPIURL
	}
	full := base + "/" + url.PathEscape(strings.TrimSpace(alias))
	if _, err := url.ParseRequestURI(full); err != nil {
		return "", fmt.Errorf("invalid community config URL %q: %w", full, err)
	}
	return full, nil
}

// extractCommunityConfig returns the community config from a communities-API
// response. The endpoint returns an envelope {alias, chain_id, json: {..config..},
// active, ...}; the config (community, tokens, accounts, chains, plugins, ...)
// lives under "json". A bare config object is also accepted. An unknown alias
// yields a bare null (or a null json field), reported as not found.
func extractCommunityConfig(body []byte) ([]byte, error) {
	body = bytes.TrimSpace(body)
	inner := unwrapEnvelope(body)
	// unwrapEnvelope returns body unchanged for a bare config (has "community"),
	// for a null body, or for an envelope whose "json" is null. Only the bare
	// config is usable; the null cases are an unknown/empty alias.
	if bytes.Equal(inner, body) && !hasTopLevelCommunity(body) {
		return nil, fmt.Errorf("community config not found (unknown alias or empty 'json' field)")
	}
	return inner, nil
}

func hasTopLevelCommunity(body []byte) bool {
	var probe struct {
		Community json.RawMessage `json:"community"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(body), &probe); err != nil {
		return false
	}
	return len(bytes.TrimSpace(probe.Community)) > 0
}

// unwrapEnvelope returns the inner config when body is a communities-API envelope
// ({alias, chain_id, json: {..config..}, ...}); otherwise it returns body
// unchanged. This lets both the API response and a local config file use the
// envelope shape. A bare config object (with a top-level "community") is passed
// through untouched.
func unwrapEnvelope(body []byte) []byte {
	var probe struct {
		Community json.RawMessage `json:"community"`
		JSON      json.RawMessage `json:"json"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(body), &probe); err != nil {
		return body
	}
	// A direct config has a top-level "community"; an envelope has "json" and none.
	if len(bytes.TrimSpace(probe.Community)) > 0 {
		return body
	}
	inner := bytes.TrimSpace(probe.JSON)
	if len(inner) > 0 && !bytes.Equal(inner, []byte("null")) {
		return inner
	}
	return body
}

func clientConfigLocalOnly() bool {
	if truthyConfigEnv("CLIENT_CONFIG_LOCAL_ONLY") || truthyConfigEnv("CITIZEN_WALLET_CONFIG_LOCAL_ONLY") {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CLIENT_CONFIG_MODE"))) {
	case "file", "local", "local-only":
		return true
	default:
		return false
	}
}

func parse(body []byte, source string) (*Config, error) {
	body = bytes.TrimSpace(body)
	if !json.Valid(body) {
		return nil, fmt.Errorf("%s is not valid JSON", source)
	}
	// Accept both the communities-API envelope ({alias, json: {..config..}, ...})
	// and a bare config object, so a saved API response works as a local file too.
	body = unwrapEnvelope(body)

	var cfg Config
	if err := json.Unmarshal(body, &cfg); err != nil {
		return nil, fmt.Errorf("error parsing %s: %w", source, err)
	}
	envExtras, err := loadEnvironmentExtras()
	if err != nil {
		return nil, err
	}
	cfg.Extras = mergeExtras(cfg.Extras, envExtras)
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", source, err)
	}
	cfg.Extras = cfg.extrasWithConfigAuthority()
	responseBody, err := responseJSON(body, cfg.Extras)
	if err != nil {
		return nil, fmt.Errorf("error preparing %s for response: %w", source, err)
	}
	cfg.raw = responseBody
	cfg.source = source
	return &cfg, nil
}

func (c *Config) validate() error {
	if strings.TrimSpace(c.Community.Alias) == "" {
		return fmt.Errorf("community.alias is required")
	}
	if c.Community.PrimaryToken.ChainID == 0 || strings.TrimSpace(c.Community.PrimaryToken.Address) == "" {
		return fmt.Errorf("community.primary_token is required")
	}
	if c.Community.PrimaryAccountFactory.ChainID == 0 || strings.TrimSpace(c.Community.PrimaryAccountFactory.Address) == "" {
		return fmt.Errorf("community.primary_account_factory is required")
	}
	if _, err := c.PrimaryToken(); err != nil {
		return err
	}
	if _, err := c.PrimaryAccount(); err != nil {
		return err
	}
	if strings.TrimSpace(c.PrimaryRPCURL()) == "" {
		return fmt.Errorf("chains[%d].node.url is required", c.ActiveChainID())
	}
	return nil
}

func findToken(tokens map[string]Token, ref AddressRef) (Token, error) {
	if len(tokens) == 0 {
		return Token{}, fmt.Errorf("tokens map is empty")
	}
	if token, ok := tokens[compositeKey(ref)]; ok {
		return token, nil
	}
	for _, token := range tokens {
		if token.ChainID == ref.ChainID && sameAddress(token.Address, ref.Address) {
			return token, nil
		}
	}
	return Token{}, fmt.Errorf("primary token %s not found in tokens map", compositeKey(ref))
}

func findAccount(accounts map[string]Account, ref AddressRef) (Account, error) {
	if len(accounts) == 0 {
		return Account{}, fmt.Errorf("accounts map is empty")
	}
	if account, ok := accounts[compositeKey(ref)]; ok {
		return account, nil
	}
	for _, account := range accounts {
		if account.ChainID == ref.ChainID && sameAddress(account.AccountFactoryAddress, ref.Address) {
			return account, nil
		}
	}
	return Account{}, fmt.Errorf("primary account factory %s not found in accounts map", compositeKey(ref))
}

func compositeKey(ref AddressRef) string {
	return fmt.Sprintf("%d:%s", ref.ChainID, strings.ToLower(strings.TrimSpace(ref.Address)))
}

func sameAddress(left, right string) bool {
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}

func loadEnvironmentExtras() (Extras, error) {
	var extras Extras
	var err error

	if extras.HoneyTokenAddress, err = envAddress(
		"HONEY_TOKEN_ADDRESS",
		"HONEY_ADDRESS",
		"HONEY_TOKEN",
		"NEXT_PUBLIC_HONEY_TOKEN_ADDRESS",
		"NEXT_PUBLIC_HONEY_ADDRESS",
		"NEXT_PUBLIC_HONEY_TOKEN",
	); err != nil {
		return Extras{}, err
	}
	if extras.HoneyDecimals, err = envIntPtr("HONEY_DECIMALS", "NEXT_PUBLIC_HONEY_DECIMALS"); err != nil {
		return Extras{}, err
	}
	if extras.ByusdTokenAddress, err = envAddress(
		"BYUSD_TOKEN_ADDRESS",
		"BYUSD_ADDRESS",
		"BYUSD_TOKEN",
		"NEXT_PUBLIC_BYUSD_TOKEN_ADDRESS",
		"NEXT_PUBLIC_BYUSD_ADDRESS",
		"NEXT_PUBLIC_BYUSD_TOKEN",
	); err != nil {
		return Extras{}, err
	}
	if extras.ByusdDecimals, err = envIntPtr("BYUSD_DECIMALS", "NEXT_PUBLIC_BYUSD_DECIMALS"); err != nil {
		return Extras{}, err
	}
	if extras.ZapperAddress, err = envAddress(
		"ZAPPER_CONTRACT_ADDRESS",
		"ZAPPER_ADDRESS",
		"NEXT_PUBLIC_ZAPPER_CONTRACT_ADDRESS",
		"NEXT_PUBLIC_ZAPPER_ADDRESS",
	); err != nil {
		return Extras{}, err
	}
	if extras.FaucetAddress, err = envAddress("FAUCET_ADDRESS", "NEXT_PUBLIC_FAUCET_ADDRESS"); err != nil {
		return Extras{}, err
	}
	if extras.BackingAssets, err = envAddressList("BACKING_ASSETS", "NEXT_PUBLIC_BACKING_ASSETS"); err != nil {
		return Extras{}, err
	}
	if extras.ReadRPCURL, err = envURL("CLIENT_READ_RPC_URL", "RPC_URL", "NEXT_PUBLIC_RPC_URL"); err != nil {
		return Extras{}, err
	}

	return extras, nil
}

func mergeExtras(base, override Extras) Extras {
	if override.HoneyTokenAddress != "" {
		base.HoneyTokenAddress = override.HoneyTokenAddress
	}
	if override.HoneyDecimals != nil {
		base.HoneyDecimals = override.HoneyDecimals
	}
	if override.ByusdTokenAddress != "" {
		base.ByusdTokenAddress = override.ByusdTokenAddress
	}
	if override.ByusdDecimals != nil {
		base.ByusdDecimals = override.ByusdDecimals
	}
	if override.ZapperAddress != "" {
		base.ZapperAddress = override.ZapperAddress
	}
	if override.FaucetAddress != "" {
		base.FaucetAddress = override.FaucetAddress
	}
	if len(override.BackingAssets) > 0 {
		base.BackingAssets = override.BackingAssets
	}
	if override.ReadRPCURL != "" {
		base.ReadRPCURL = override.ReadRPCURL
	}
	return base
}

func (c *Config) extrasWithConfigAuthority() Extras {
	extras := c.Extras
	if hasTokenSymbol(c.Tokens, "HONEY") {
		extras.HoneyTokenAddress = ""
		extras.HoneyDecimals = nil
	}
	if hasTokenSymbol(c.Tokens, "BYUSD") {
		extras.ByusdTokenAddress = ""
		extras.ByusdDecimals = nil
	}
	return extras
}

func hasTokenSymbol(tokens map[string]Token, symbol string) bool {
	for _, token := range tokens {
		if strings.EqualFold(strings.TrimSpace(token.Symbol), symbol) {
			return true
		}
	}
	return false
}

func (e Extras) isZero() bool {
	return e.HoneyTokenAddress == "" &&
		e.HoneyDecimals == nil &&
		e.ByusdTokenAddress == "" &&
		e.ByusdDecimals == nil &&
		e.ZapperAddress == "" &&
		e.FaucetAddress == "" &&
		len(e.BackingAssets) == 0 &&
		e.ReadRPCURL == ""
}

func responseJSON(body []byte, extras Extras) ([]byte, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	rawExtras, hasExtras := doc["extras"]
	if extras.isZero() && !hasExtras {
		return append([]byte(nil), body...), nil
	}

	extrasDoc := make(map[string]json.RawMessage)
	if hasExtras && len(bytes.TrimSpace(rawExtras)) > 0 && string(bytes.TrimSpace(rawExtras)) != "null" {
		if err := json.Unmarshal(rawExtras, &extrasDoc); err != nil {
			return nil, fmt.Errorf("extras must be an object: %w", err)
		}
	}
	deleteKnownExtraFields(extrasDoc)
	writeStringExtra(extrasDoc, "honey_token_address", extras.HoneyTokenAddress)
	writeIntExtra(extrasDoc, "honey_decimals", extras.HoneyDecimals)
	writeStringExtra(extrasDoc, "byusd_token_address", extras.ByusdTokenAddress)
	writeIntExtra(extrasDoc, "byusd_decimals", extras.ByusdDecimals)
	writeStringExtra(extrasDoc, "zapper_address", extras.ZapperAddress)
	writeStringExtra(extrasDoc, "faucet_address", extras.FaucetAddress)
	writeStringExtra(extrasDoc, "rpc_url", extras.ReadRPCURL)
	writeStringSliceExtra(extrasDoc, "backing_assets", extras.BackingAssets)

	if len(extrasDoc) == 0 {
		delete(doc, "extras")
		return json.Marshal(doc)
	}
	rawExtras, err := json.Marshal(extrasDoc)
	if err != nil {
		return nil, err
	}
	doc["extras"] = rawExtras
	return json.Marshal(doc)
}

func deleteKnownExtraFields(doc map[string]json.RawMessage) {
	for _, key := range []string{
		"honey_token_address",
		"honeyTokenAddress",
		"honey_address",
		"honeyAddress",
		"honey_decimals",
		"honeyDecimals",
		"byusd_token_address",
		"byusdTokenAddress",
		"byusd_address",
		"byusdAddress",
		"byusd_decimals",
		"byusdDecimals",
		"zapper_address",
		"zapperAddress",
		"zapper_contract_address",
		"zapperContractAddress",
		"faucet_address",
		"faucetAddress",
		"backing_assets",
		"backingAssets",
		"rpc_url",
		"rpcUrl",
	} {
		delete(doc, key)
	}
}

func writeStringExtra(doc map[string]json.RawMessage, key, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	raw, _ := json.Marshal(strings.TrimSpace(value))
	doc[key] = raw
}

func writeIntExtra(doc map[string]json.RawMessage, key string, value *int) {
	if value == nil {
		return
	}
	raw, _ := json.Marshal(*value)
	doc[key] = raw
}

func writeStringSliceExtra(doc map[string]json.RawMessage, key string, value []string) {
	if len(value) == 0 {
		return
	}
	raw, _ := json.Marshal(value)
	doc[key] = raw
}

func envURL(names ...string) (string, error) {
	name, value := firstEnv(names...)
	if value == "" {
		return "", nil
	}
	if _, err := url.ParseRequestURI(value); err != nil {
		return "", fmt.Errorf("%s must be a valid URL: %w", name, err)
	}
	return value, nil
}

func envAddress(names ...string) (string, error) {
	name, value := firstEnv(names...)
	if value == "" {
		return "", nil
	}
	if !isHexAddress(value) {
		return "", fmt.Errorf("%s must be an EVM address", name)
	}
	return value, nil
}

func envAddressList(names ...string) ([]string, error) {
	name, value := firstEnv(names...)
	if value == "" {
		return nil, nil
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\t' || r == ' '
	})
	addresses := make([]string, 0, len(parts))
	for _, part := range parts {
		address := strings.TrimSpace(part)
		if address == "" {
			continue
		}
		if !isHexAddress(address) {
			return nil, fmt.Errorf("%s contains invalid EVM address %q", name, address)
		}
		addresses = append(addresses, address)
	}
	return addresses, nil
}

func envIntPtr(names ...string) (*int, error) {
	name, value := firstEnv(names...)
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return nil, fmt.Errorf("%s must be a non-negative integer", name)
	}
	return &parsed, nil
}

func firstEnv(names ...string) (string, string) {
	for _, name := range names {
		value := strings.TrimSpace(os.Getenv(name))
		if value != "" {
			return name, value
		}
	}
	return "", ""
}

func truthyConfigEnv(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "t", "true", "y", "yes", "on":
		return true
	default:
		return false
	}
}

func isHexAddress(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 42 || !strings.HasPrefix(strings.ToLower(value), "0x") {
		return false
	}
	for _, r := range value[2:] {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}
