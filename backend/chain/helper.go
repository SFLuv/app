package chain

import (
	"fmt"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/contract"
)

var maybeTxOutput func(string)

func TxnDoWait(txn contract.Txn, errIn error) error {
	if errIn != nil {
		return errIn
	}
	if err := txn.Do(); err != nil {
		return err
	} else {
		var rcpt *ethgo.Receipt
		if rcpt, err = txn.Wait(); err != nil {
			return err
		}
		if maybeTxOutput != nil {
			maybeTxOutput(fmt.Sprintf("transaction succeeded: hash %v, gas %v, cnt logs %v",
				rcpt.TransactionHash.String(), rcpt.GasUsed, len(rcpt.Logs)))
		}
	}
	return nil
}
