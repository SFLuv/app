import { getAddress, isAddress, type Address } from "viem";
import { ADMIN_ADDRESS, SFLUV_TOKEN } from "./constants";
import { AppWallet } from "./wallets/wallets";

const SWEEP_TIMEOUT_MS = 120_000;
const SWEEP_POLL_INTERVAL_MS = 2_000;

async function waitForWalletBalanceAtMost(
  wallet: AppWallet,
  maxBalance: bigint = 0n,
): Promise<boolean> {
  const startedAt = Date.now();

  while (Date.now() - startedAt < SWEEP_TIMEOUT_MS) {
    const balance = await wallet.getBalance(SFLUV_TOKEN);
    if (balance !== null && balance <= maxBalance) {
      return true;
    }
    await new Promise((resolve) => setTimeout(resolve, SWEEP_POLL_INTERVAL_MS));
  }

  return false;
}

export function resolveAccountDeletionAdminAddress(): Address {
  if (!ADMIN_ADDRESS || !isAddress(ADMIN_ADDRESS)) {
    throw new Error("The account-deletion transfer destination is not configured.");
  }

  return getAddress(ADMIN_ADDRESS);
}

export async function sweepSFLUVBalancesToAdmin(
  wallets: AppWallet[],
): Promise<{ checkedWallets: number; transferredWallets: number }> {
  const adminAddress = resolveAccountDeletionAdminAddress();
  const uniqueWallets = new Map<string, AppWallet>();

  for (const wallet of wallets) {
    if (!wallet.address) {
      continue;
    }
    uniqueWallets.set(wallet.address.toLowerCase(), wallet);
  }

  let checkedWallets = 0;
  let transferredWallets = 0;

  for (const wallet of uniqueWallets.values()) {
    if (!wallet.address || wallet.address.toLowerCase() === adminAddress.toLowerCase()) {
      continue;
    }

    checkedWallets += 1;

    const balance = await wallet.getBalance(SFLUV_TOKEN);
    if (balance === null) {
      throw new Error(`Unable to read the SFLUV balance for ${wallet.name}.`);
    }
    if (balance <= 0n) {
      continue;
    }

    if (wallet.type === "smartwallet") {
      const deployed = await wallet.ensureSmartWalletDeployed();
      if (!deployed) {
        throw new Error(`Unable to ready ${wallet.name} for transfer.`);
      }
    }

    const receipt = await wallet.send(balance, adminAddress);
    if (!receipt) {
      throw new Error(`Unable to start the transfer for ${wallet.name}.`);
    }
    if (receipt.error || !receipt.hash) {
      throw new Error(receipt.error || `Unable to transfer SFLUV from ${wallet.name}.`);
    }

    const cleared = await waitForWalletBalanceAtMost(wallet);
    if (!cleared) {
      throw new Error(`The transfer from ${wallet.name} is still pending.`);
    }

    transferredWallets += 1;
  }

  return {
    checkedWallets,
    transferredWallets,
  };
}
