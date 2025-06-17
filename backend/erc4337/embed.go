package erc4337

import (
	"embed"
)

//go:embed abi/IEntryPoint.json
var abiIEP embed.FS

//go:embed abi/SFLUVv1.json
var abiSFLUV embed.FS
