package handlers

import (
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

func (a *AppService) ProcessHook(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		fmt.Printf("error reading alchemy hook body: %s\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if !utils.WebhookAuth(r, body) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	fmt.Println(body)

	// var hook structs.AlchemyHook
	// err = json.Unmarshal(body, &hook)
	// if err != nil {
	// 	fmt.Printf("error unmarshalling alchemy hook body: %s\n", err)
	// 	w.WriteHeader(http.StatusInternalServerError)
	// 	return
	// }

	// switch hook.

	// d, err := strconv.Atoi(os.Getenv("SFLUV_DECIMALS"))
	// if err != nil {
	// 	return 0, fmt.Errorf("error getting sfluv decimals from env: %s", err)
	// }
}

// d, err := strconv.Atoi(os.Getenv("SFLUV_DECIMALS"))
// if err != nil {
// 	return 0, fmt.Errorf("error getting sfluv decimals from env: %s", err)
// }

// func formatTransaction(tx *structs.AlchemyTx) []*structs.FormattedTransaction {

// }

func _getValue(tx *structs.AlchemyTx, decimals int) (int64, error) {
	i := new(big.Int)
	amount, ok := i.SetString(tx.Log.Data, 0)
	if !ok {
		return 0, fmt.Errorf("error parsing bigint from tx log data")
	}

	d := big.NewInt(int64(math.Pow10(decimals)))

	v := new(big.Int)
	value := v.Div(amount, d).Int64()

	return value, nil
}
