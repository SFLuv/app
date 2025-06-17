package crypto

import (
	"encoding/hex"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo/wallet"
	"os"
	"testing"
)

// TODO: prob should just delete ...
func noTestRecover(t *testing.T) {

	sk, err := SKFromHex(os.Getenv("USEROP_SK"))
	require.NoError(t, err)

	testHashHex := "90cd3556db7a1a107c9edddd430b9bbbf7ae0e5d3829f2bb4e509fca64d45a90"
	testHash, _ := hex.DecodeString(testHashHex)

	sig, err := Sign(sk, testHash)
	require.NoError(t, err)
	println(hexutil.Encode(sig))

	testAddr, err := wallet.Ecrecover(testHash, sig)
	require.NoError(t, err)
	println(testAddr.String())

	testSigHex := "4a26c0995ac0f7a3006c24e57345f824628b327530522e41f50c3ef43d28754534c7e9cc98fbbdc437950ac2ade4f033507dc219365ae53638676aa8b47608ad1c"
	testSig, _ := hex.DecodeString(testSigHex)

	if testSig[64] >= 27 {
		testSig[64] -= 27
	}
	checkAddr, err := wallet.Ecrecover(testHash, testSig)
	require.NoError(t, err)
	println(checkAddr.String())

}
