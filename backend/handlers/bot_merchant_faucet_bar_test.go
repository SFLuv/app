package handlers

import (
	"testing"

	"github.com/SFLuv/app/backend/structs"
)

// The bar is identity shaped on purpose. resolveRedeemPayoutAddress has already
// rewritten a location till to the owner's personal wallet by the time this
// runs, so a check that looked at the address would be looking at a wallet that
// belongs to a person and would never fire.
func TestMerchantFaucetRefusal(t *testing.T) {
	tests := []struct {
		name    string
		enabled string
		target  redeemPayoutTarget
		want    string
	}{
		{
			name:   "merchant account is refused",
			target: redeemPayoutTarget{userID: "did:privy:shop", accountType: structs.AccountTypeMerchant},
			want:   redeemRefusalMerchantAccount,
		},
		{
			name:   "regular account is paid",
			target: redeemPayoutTarget{userID: "did:privy:volunteer", accountType: structs.AccountTypeRegular},
			want:   "",
		},
		{
			name:   "an unreadable owner fails closed",
			target: redeemPayoutTarget{userID: "did:privy:shop", ownerUnreadable: true},
			want:   redeemRefusalOwnerUnreadable,
		},
		{
			// The known hole: an anonymous scan from a wallet nobody has linked.
			name:   "an unowned address is paid",
			target: redeemPayoutTarget{},
			want:   "",
		},
		{
			name:    "the flag turns the whole thing off",
			enabled: "false",
			target:  redeemPayoutTarget{userID: "did:privy:shop", accountType: structs.AccountTypeMerchant},
			want:    "",
		},
		{
			name:    "a disabled bar also stops failing closed",
			enabled: "false",
			target:  redeemPayoutTarget{userID: "did:privy:shop", ownerUnreadable: true},
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(merchantFaucetBarEnvKey, tt.enabled)

			got := merchantFaucetRefusal(tt.target)
			if got != tt.want {
				t.Fatalf("merchantFaucetRefusal(%+v) = %q; want %q", tt.target, got, tt.want)
			}
		})
	}
}

// Absent configuration must bar, not pay. A deployment that has never heard of
// this flag is the common case and it is the one that has to be safe.
func TestMerchantFaucetBarDefaultsOn(t *testing.T) {
	t.Setenv(merchantFaucetBarEnvKey, "")

	if !merchantFaucetBarEnabled() {
		t.Fatal("merchant faucet bar is off with no configuration; it must default on")
	}
}
