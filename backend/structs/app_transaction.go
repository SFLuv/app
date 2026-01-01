package structs

type TransactionType string
type AlchemyRpcMethod string
type AlchemyAssetTransferCategory string

const (
	Purchase TransactionType = "purchase"
	Unwrap   TransactionType = "unwrap"
	Reward   TransactionType = "reward"
	Transfer TransactionType = "transfer"
)

const (
	GetAssetTransfers AlchemyRpcMethod = "alchemy_getAssetTransfers"
)

const (
	External   AlchemyAssetTransferCategory = "external"
	Internal   AlchemyAssetTransferCategory = "internal"
	Erc20      AlchemyAssetTransferCategory = "erc20"
	Erc721     AlchemyAssetTransferCategory = "erc721"
	Erc1155    AlchemyAssetTransferCategory = "erc1155"
	SpecialNft AlchemyAssetTransferCategory = "specialnft"
)

type AlchemyRpcRequest struct {
	Id      uint64 `json:"id"`
	Version string `json:"jsonrpc"`
	Method  string `json:"method"`
}

type AlchemyRpcResponse struct {
	Id      uint64 `json:"id"`
	Version string `json:"jsonrpc"`
	Method  string `json:"method"`
}

type AlchemyGetTransfersRequest struct {
	AlchemyRpcRequest
	Params []AlchemyGetAssetTransfersParams `json:"params"`
}

type AlchemyGetTransfersResponse struct {
	AlchemyRpcResponse
	Result AlchemyGetTransfersResult `json:"result"`
}

type AlchemyGetTransfersResult struct {
	Transfers []AlchemyTransferResponse `json:"transfers"`
	PageKey   string                    `json:"pageKey"`
}

type AlchemyGetAssetTransfersParams struct {
	FromBlock        string                       `json:"fromBlock"`
	ToBlock          string                       `json:"toBlock"`
	FromAddress      string                       `json:"fromAddress"`
	ToAddress        string                       `json:"toAddress"`
	ExcludeZeroValue string                       `json:"excludeZeroValue"`
	WithMetadata     string                       `json:"withMetadata"`
	Category         AlchemyAssetTransferCategory `json:"category"`
	PageKey          string                       `json:"pageKey"`
}

type AlchemyTransferResponse struct {
	BlockNum        string                       `json:"blockNum"`
	UniqueId        string                       `json:"uniqueId"`
	Hash            string                       `json:"hash"`
	From            string                       `json:"from"`
	To              string                       `json:"to"`
	Value           float64                      `json:"value"`
	Erc721TokenId   string                       `json:"erc721TokenId"`
	Erc1155Metadata AlchemyErc1155Metadata       `json:"erc1155Metadata"`
	TokenId         string                       `json:"tokenId"`
	Asset           string                       `json:"asset"`
	Category        AlchemyAssetTransferCategory `json:"category"`
	RawContract     AlchemyRawContract           `json:"rawContract"`
}

type AlchemyErc1155Metadata struct {
	TokenId string `json:"tokenId"`
	Value   string `json:"value"`
}

type AlchemyRawContract struct {
	Value   string `json:"value"`
	Address string `json:"contract"`
	Decimal string `json:"decimal"`
}

type FormattedTransaction struct {
	Id           string          `json:"id"`
	Wallet       string          `json:"wallet"`
	Direction    string          `json:"direction"`
	Counterparty string          `json:"counterparty"`
	Type         TransactionType `json:"type"`
	Amount       float64         `json:"amount"`
	Timestamp    uint64          `json:"timestamp"`
}
